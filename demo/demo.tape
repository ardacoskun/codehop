# Renders assets/demo.gif. Run from the repo root:
#   demo/setup.sh /tmp/codehop-demo && vhs demo/demo.tape
Output assets/demo.gif

Set Shell zsh
Set FontSize 18
Set Width 1200
Set Height 640
Set Padding 24
Set Theme "Catppuccin Mocha"
Set TypingSpeed 70ms
Env HOME "/tmp/codehop-demo/home"
Env PATH "/tmp/codehop-demo/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
Env ZDOTDIR "/tmp/codehop-demo/home"
Env FZF_DEFAULT_OPTS "--color=bg+:#313244,pointer:#cba6f7,hl:#f38ba8,hl+:#f38ba8"

# vhs starts zsh without rc files, so load the demo setup explicitly. The
# demo bin dir comes first on PATH, so nothing reaches a real editor.
Hide
Type "source /tmp/codehop-demo/home/.zshrc && cd ~ && clear" Enter
Sleep 500ms
Show

# Tab-complete a name
Type "code de" Sleep 400ms Tab Sleep 900ms Enter Sleep 1.5s

# Fuzzy: a word in the middle of the name
Type "code shuffle" Sleep 400ms Enter Sleep 1.5s

# Several matches: the picker opens with just those, preview included
Type "code serv" Sleep 300ms Enter Sleep 2s
Down Sleep 1.2s Enter Sleep 1.5s

# No argument: pick from every project
Type "code" Sleep 300ms Enter Sleep 2s
Type "high" Sleep 1.2s Enter Sleep 1.5s

# Jump into a project and check every repo at once
Type "cdp api" Sleep 300ms Enter Sleep 600ms
Type "pwd" Enter Sleep 1s
Type "codehop status" Enter Sleep 3s
