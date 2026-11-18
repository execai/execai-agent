# execai demo — recorded with VHS: `vhs scripts/demo.tape`
# https://github.com/charmbracelet/vhs
#
# Scenario (~70s):
#   1. Boot execai (default source)
#   2. /source — provider picker opens (9 sources), filter to kimi, switch
#   3. Status bar shows kimi (K3 + HighSpeed) — auto-detected tier
#   4. Real agentic task: check a live Kubernetes cluster
#   5. /quit
#
# Before recording: set credentials email to it@execai.ru and active
# source to execai (see notes in the repo memory). Output is embedded
# in all 5 READMEs.

Output "scripts/demo.gif"

Set FontSize 14
Set Width 1200
Set Height 760
Set Padding 16
Set Framerate 24
Set TypingSpeed 55ms
Set Theme "Catppuccin Mocha"

# --- Boot ---
Type "execai"
Enter
Sleep 2.5s

# --- Provider picker: show all 9 sources ---
Type "/source "
Sleep 2.5s

# Filter down to kimi and switch
Type "kimi"
Sleep 1.5s
Enter
Sleep 2.5s

# --- Real agentic task on a live cluster ---
Type "check the kubernetes cluster: are all pods healthy? how many nodes?"
Sleep 600ms
Enter
Sleep 26s

# Let the final answer sit for a beat
Sleep 3s

# --- Exit ---
Type "/quit"
Enter
Sleep 800ms
