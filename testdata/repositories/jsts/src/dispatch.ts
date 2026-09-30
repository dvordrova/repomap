// A command line read by comparing its first word: the switch on the
// program's argument compares one value with each subcommand's name
// ("check" and "verify" stacked on one body are one case), and its default
// compares the same value with the help words, one question for the whole
// dispatch. A lone comparison (isDefault) is no dispatch.
export function dispatch(): string {
  switch (process.argv[2]) {
    case "build":
      return runBuild()
    case "check":
    case "verify":
      return "check"
    default:
      if (process.argv[2] === "help" || process.argv[2] === "-h") return "usage"
      return "unknown"
  }
}

export function isDefault(level: string): boolean {
  return level === "default"
}

// build's own code: only the build case runs it, so build is handled there.
function runBuild(): string {
  return "build"
}
