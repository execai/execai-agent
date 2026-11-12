# Retake: only the missing scenes — K3 answer + /usage (long waits).
Output "/tmp/article-shots2.gif"

Set FontSize 15
Set Width 1100
Set Height 640
Set Padding 14
Set Framerate 8
Set TypingSpeed 40ms
Set Theme "Catppuccin Mocha"

Type "execai"
Enter
Sleep 3s

# K3 answer (60s budget — effort:max thinks long)
Type "объясни в двух предложениях зачем нужен interface в Go"
Sleep 500ms
Enter
Sleep 60s

# /usage — live Kimi quotas
Type "/usage"
Enter
Sleep 10s

Type "/quit"
Enter
Sleep 500ms
