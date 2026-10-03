// This module names its own functions like a setting read and code
// evaluation. A call to them runs this repository's code, whatever the name:
// it reads no setting and evaluates nothing. (A module cannot declare a
// function named eval: it is strict code.)
export function getenv(key: string): string {
  return "database row: " + key
}

export function exec(statement: string): string {
  return statement
}

export function readLookalikes(): string[] {
  return [getenv("CUSTOMER_ROW"), exec("show status")]
}

// The platform's own constructor of a function from source text.
export function compileBody(body: string): unknown {
  return new Function(body)()
}

// A RegExp's exec matches text; it evaluates no code.
export function firstDigits(text: string): string {
  return /\d+/.exec(text)?.[0] ?? ""
}
