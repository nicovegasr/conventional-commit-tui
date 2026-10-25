# The /stats screen. Run: vhs assets/tapes/stats.tape
Source assets/tapes/theme.tape
Output assets/stats.gif
Set Width 1500
Set Height 440

Hide
Type "source assets/tapes/setup.sh" Enter
Sleep 4s
Show

Type "nib" Enter
Sleep 2s
Type "/st"
Sleep 800ms
Enter
Sleep 1.5s
Screenshot assets/stats.png
Sleep 1s
