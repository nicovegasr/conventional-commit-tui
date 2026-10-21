package history

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Parse consumes Git's NUL-delimited raw+numstat stream. Tokens are bounded at
// 16 MiB; oversize messages fail explicitly rather than truncating the index.
func Parse(r io.Reader, visit func(Commit) error) error {
	p := parser{scan: bufio.NewScanner(r)}
	p.scan.Split(splitNUL)
	p.scan.Buffer(make([]byte, 4096), 16*1024*1024)
	for p.scan.Scan() {
		token := strings.TrimLeft(p.scan.Text(), "\n")
		if token == "" {
			continue
		}
		if token == "nib-commit" {
			if err := p.flush(visit); err != nil {
				return err
			}
			if err := p.header(); err != nil {
				return err
			}
		} else if err := p.change(token); err != nil {
			return err
		}
	}
	if err := p.scan.Err(); err != nil {
		return err
	}
	return p.flush(visit)
}

func splitNUL(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		if len(bytes.Trim(data, "\n")) == 0 {
			return len(data), nil, nil
		}
		return 0, nil, errors.New("truncated Git history token")
	}
	return 0, nil, nil
}

type parser struct {
	scan      *bufio.Scanner
	commit    Commit
	positions map[string]int
	stats     map[string]bool
}

func (p *parser) next() (string, error) {
	if p.scan.Scan() {
		return p.scan.Text(), nil
	}
	if err := p.scan.Err(); err != nil {
		return "", err
	}
	return "", io.ErrUnexpectedEOF
}

func (p *parser) header() error {
	fields := make([]string, 6)
	for i := range fields {
		s, err := p.next()
		if err != nil {
			return err
		}
		fields[i] = s
	}
	timestamp, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid commit timestamp: %w", err)
	}
	if !validOID(fields[0]) {
		return errors.New("invalid commit OID")
	}
	p.commit = Commit{OID: fields[0], Parents: strings.Fields(fields[1]), Author: fields[2], Email: fields[3], Timestamp: timestamp, Message: fields[5]}
	p.positions = map[string]int{}
	p.stats = map[string]bool{}
	return nil
}

func validOID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

func (p *parser) flush(visit func(Commit) error) error {
	if p.commit.OID == "" {
		return nil
	}
	if len(p.stats) != len(p.commit.Changes) {
		return errors.New("incomplete Git line statistics")
	}
	return visit(p.commit)
}

func (p *parser) change(token string) error {
	if p.commit.OID == "" {
		return errors.New("change before commit header")
	}
	if strings.HasPrefix(token, ":") {
		return p.raw(token)
	}
	return p.numstat(token)
}

func (p *parser) raw(token string) error {
	fields := strings.Fields(token)
	if len(fields) != 5 || fields[4] == "" {
		return errors.New("invalid raw change")
	}
	path, err := p.next()
	if err != nil {
		return err
	}
	c := Change{Path: path, Kind: fields[4][:1]}
	if c.Kind == "R" || c.Kind == "C" {
		c.PreviousPath = path
		c.Path, err = p.next()
		if err != nil {
			return err
		}
	}
	if c.Path == "" {
		return errors.New("empty changed path")
	}
	if !utf8.ValidString(c.Path) || !utf8.ValidString(c.PreviousPath) {
		return errors.New("non-UTF-8 paths are not supported by this index schema")
	}
	if _, exists := p.positions[c.Path]; exists {
		return errors.New("duplicate changed path")
	}
	p.positions[c.Path] = len(p.commit.Changes)
	p.commit.Changes = append(p.commit.Changes, c)
	return nil
}

func (p *parser) numstat(token string) error {
	fields := strings.SplitN(token, "\t", 3)
	if len(fields) != 3 {
		return fmt.Errorf("invalid numstat token %q", token)
	}
	path := fields[2]
	if path == "" {
		if _, err := p.next(); err != nil {
			return err
		}
		var err error
		path, err = p.next()
		if err != nil {
			return err
		}
	}
	i, ok := p.positions[path]
	if !ok || p.stats[path] {
		return errors.New("numstat path does not match raw change")
	}
	c := &p.commit.Changes[i]
	if fields[0] == "-" && fields[1] == "-" {
		c.Binary = true
	} else {
		added, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil || added < 0 {
			return errors.New("invalid added line count")
		}
		deleted, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || deleted < 0 {
			return errors.New("invalid deleted line count")
		}
		c.Added, c.Deleted = added, deleted
	}
	p.stats[path] = true
	return nil
}
