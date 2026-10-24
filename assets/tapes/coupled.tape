# `nib coupled` from the CLI. Run: vhs assets/tapes/coupled.tape
Source assets/tapes/theme.tape
Output assets/coupled.gif
Set Width 1400
Set Height 700

Hide
Type "source assets/tapes/setup.sh" Enter
Sleep 4s
Show

Type "nib coupled internal/ui/model.go --limit 4 --evidence 2" Enter
Sleep 1.5s
Screenshot assets/coupled.png
Sleep 1s
