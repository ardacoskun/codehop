package config

// Template is written by `codehop config init`. Every setting is commented
// out so the file documents the defaults without overriding them.
const Template = `# codehop configuration — https://github.com/ardacoskun/codehop
# Every setting is optional; the commented values are the defaults.

# Folders searched for projects. Default: whichever of ~/projects, ~/code,
# ~/src, ~/dev, ~/Developer, ~/repos, ~/workspace, ~/git exist.
# roots = ["~/projects"]

# How many levels below a root to look. A folder that contains a marker is a
# project and is not searched further.
# max_depth = 3

# Files or folders (globs allowed) that make a folder a project.
# markers = [".git", ".codehop", "package.json", "go.mod", "Cargo.toml", "pyproject.toml",
#            "pubspec.yaml", "composer.json", "Gemfile", "pom.xml", "build.gradle",
#            "build.gradle.kts", "Package.swift", "deno.json", "mix.exs", "*.sln",
#            "*.code-workspace"]

# Folder names never searched.
# ignore = ["node_modules", "vendor", "dist", "build", "target", "Pods",
#           "DerivedData", "venv", ".venv", "__pycache__"]

# Editor: a preset (code, code-insiders, cursor, windsurf, codium, zed, subl,
# idea, webstorm, goland, pycharm, nvim, vim, hx) or any command. Use {path} to
# place the project path; otherwise it is appended.
# editor = "code"

# Open the project's single *.code-workspace file instead of the folder
# (VS Code-family editors only). Skipped when the file lists folders outside
# the project.
# prefer_workspace = true

# Where "code gh:owner/repo" clones repositories. Default: the first root.
# clone_root = "~/projects"

# Extra fzf options for the picker.
# fzf_opts = "--height=50%"

# Actions become shell commands that cd into a project and run something.
# They are inserted verbatim into your shell, e.g. "cc color-shuffle".
# [actions]
# cc = "claude"
# t = "npm test"
`
