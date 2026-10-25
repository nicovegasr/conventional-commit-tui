# The /changes diff screen. Run: vhs assets/tapes/changes.tape
Source assets/tapes/theme.tape
Output assets/changes.gif
Set Width 1500
Set Height 760

Hide
Type "source assets/tapes/setup.sh" Enter
Sleep 4s
Type `sed -i.bak 's/const weeklyBeyond = 30/const weeklyBeyond = 31/; s|// weeklyBeyond is the widest window still shown per day.|// weeklyBeyond is the widest window still shown per day: a month fits.|' internal/stats/stats.go && rm internal/stats/stats.go.bak && clear` Enter
Sleep 1s
Show

Type "nib" Enter
Sleep 2s
Type "/ch"
Sleep 800ms
Enter
Sleep 1s
Type "/"
Type "stats"
Sleep 500ms
Enter
Sleep 1.5s
Screenshot assets/changes.png
Sleep 1s
