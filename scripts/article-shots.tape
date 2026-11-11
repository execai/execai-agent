# Screenshots for the Habr article — frames extracted from this GIF via ffmpeg.
# Scenes: /source picker → switch to kimi → real K3 answer → /usage quotas → /model list.
Output "/tmp/article-shots.gif"

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

# Scene 1: /source picker (пауза подольше — кадр)
Type "/source "
Sleep 4s

# Scene 2: переключение на kimi
Type "kimi"
Sleep 1s
Enter
Sleep 3.5s

# Scene 3: реальный вопрос к K3
Type "объясни в двух предложениях зачем нужен interface в Go"
Sleep 500ms
Enter
Sleep 45s

# Scene 4: /usage — живые квоты Kimi Code
Type "/usage"
Enter
Sleep 8s

# Scene 5: /model список
Type "/model"
Enter
Sleep 6s

Type "/quit"
Enter
Sleep 500ms
