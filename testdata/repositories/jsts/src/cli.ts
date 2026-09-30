// A command-line tool: commander declares its option and its subcommand.
// .option is given the option's words; .action hands the handler over, so
// the same chain holds one call given words and one handed a callable.
import {Command} from "commander"
import {spawn} from "node:child_process"

export function run(options: {port?: string}): void {
  console.log("serving on", options.port)
}

export function initProject(): void {}

export const program = new Command()
program.option("-p, --port <n>", "port to listen on").action(run)
program.command("init").action(initProject)

// A command group: remote is a word a person types before its own
// subcommands, add and rm, whose actions hand their handlers over on each
// subcommand's own result. The word is the group's own, given by position,
// so remote stays an input beside them (GroupsIndex keeps a group word).
export function addRemote(): void {}

export function removeRemote(): void {}

export const remote = program.command("remote")
remote.command("add").action(addRemote)
remote.command("rm").action(removeRemote)

// Starts another program, git, with its subcommand: its words are git's.
export function revision(): void {
  spawn("git", ["rev-parse", "HEAD"], {stdio: "inherit"})
}
