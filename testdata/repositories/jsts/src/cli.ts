// A command-line tool: commander declares its option and its subcommand.
// .option is given the option's words; .action hands the handler over, so
// the same chain holds one call given words and one handed a callable.
import {Command} from "commander"

export function run(options: {port?: string}): void {
  console.log("serving on", options.port)
}

export function initProject(): void {}

export const program = new Command()
program.option("-p, --port <n>", "port to listen on").action(run)
program.command("init").action(initProject)
