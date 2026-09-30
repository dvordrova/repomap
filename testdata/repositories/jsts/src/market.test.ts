export function exerciseMarket(): string {
  return "configured test fixture";
}

// SQL a test runs is the test's, not the program's: its table reaches no data
// and its call no outbound call of the program.
export function createTestOnlyRows(execute: (statement: string) => void): void {
  execute("CREATE TABLE test_only_rows (id INTEGER PRIMARY KEY)");
}

// A test hands the program's throttle an arrow function: none of the
// program's values.
import { Throttle } from "./stored-callbacks";

export function throttleAnArrow(): void {
  new Throttle().throttle(() => {}, 0);
}
