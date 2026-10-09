// The API returns a count, not a list of level objects.
export interface IGetLevelsResponse {
  count: number;
}

// Same-named fields retain their own interface and written modifiers.
export interface OtherResponse {
  readonly count?: string;
  metadata?: { // This is a nested field, not another interface member.
    nestedOnly: boolean
  };
  callback?: () => void;
  readonly source: "node_modules/pkg  internal";
  message: `first
second`;
  route: `first
${number}  last`;
}

// Inherited fields are not new declarations owned by the derived interface.
export interface ExtendedResponse<T extends { id: string } = { id: string }> extends IGetLevelsResponse {
  own: boolean;
}

// Anonymous type-literal members and runtime properties stay outside this slice.
export type LiteralResponse = { aliasOnly: number };
const runtimeValue = { runtimeOnly: 1 };
runtimeValue.runtimeOnly = 2;

// A generic class keeps its whole type-parameter list; its fields are not header text.
export class Box<T extends { id: string }> {
  value?: T;
}

// A generic alias keeps its parameter list, not its body.
export type Pair<T> = { first: T; second: T };

// An alias's parameters keep their constraints and defaults.
export type Keyed<K extends string, V = number> = Record<K, V>;

export function firstOf<T>(items: T[]): T {
  return items[0];
}

// Overload signatures repeat one name: the map of parts reads them and the
// implementation as one unit, shown with the first signature.
export function pick(items: string[]): string;
export function pick(items: number[]): number;
/** The implementation counts its own lines of code; this JSDoc, the comment
 * inside and the blank line are none of them. */
export function pick(items: Array<string | number>): string | number {
  // The first item is the pick.

  return items[0];
}

// TODO: document count validation.

// NOTE: count describes a quantity, not a list of levels.

// A method's and a constructor's overload signatures fold into their
// implementation as pick's do: one declaration each, and a call of the
// overloaded name reaches the implementation alone.
export class Picker {
  constructor(prefix: string);
  constructor(prefix: number);
  constructor(readonly prefix: string | number) {}

  choose(items: string[]): string;
  choose(items: number[]): number;
  choose(items: Array<string | number>): string | number {
    return pick(items as string[]);
  }
}

export function pickAll(picker: Picker): Array<string | number> {
  return [picker.choose(["a"]), pick([1]), new Picker("p").prefix];
}

/**
 * Serialize a pair while retaining its two declared types.
 * The alias preserves both named fields.
 *
 * @example
 * ```ts
 * type Input = Pair<string>;
 * type Output = SerializedPair<Input>;
 * const first: Output["first"] = "level";
 * const second: Output["second"] = "next";
 * ```
 *
 * @remarks
 * This is a type declaration, not a runtime serializer.
 * The example is documentation, not another declaration in the file.
 * Its comment belongs to this alias even when the example makes the
 * block longer than a nearby-comment heuristic would permit.
 *
 * @category Types
 */
export type SerializedPair<T> = { first: T; second: T };
export type UndocumentedPair<T> = { value: T };

// These compiler type strings are complete words, not the variable names.
export const a: any = null;
export const i: number = 1;

/** Counts an authored level. */
export const
  authoredCount = 1;

/** Read the authored count. */
export function
  readAuthoredCount(): number { return authoredCount; }

export function undocumentedCount(): number { return authoredCount; }

/** A blank line still leaves compiler-owned documentation. */

export type AuthoredAfterBlank = number;
export type UndocumentedAfterBlank = number;

/** Names the authored response shape. */
export interface AuthoredShape {
  /** Counts response levels. */
  readonly count:
    number;
  next: number;
}

/** Holds two contrasting methods. */
export class AuthoredReader {
  /** Reads the authored level. */
  read(): number { return authoredCount; }
  other(): number { return authoredCount; }
}

/** Describes two variables in one source declaration. */
export const
  sharedCommentFirst = 1,
  sharedCommentSecond = 2;

/** Counts inline levels. */ export const inlineOwned = 1;
export const inlineUndocumented = 2;
/** Reads the next independent level. */
export const afterInlineOwned = 3;
export const afterInlineUndocumented = 4;

/** Describes the first same-line variable. */
export const sameLineListFirst = 1, sameLineListSecond = 2;
export const /** Counts a compiler-owned inner comment. */ insideDeclarationOwned = 1;
export const /** Describes the first inline declaration. */ inlinePairFirst = 1; export const /** Describes the second inline declaration. */ inlinePairSecond = 2;
export const unicodeLead = "😀"; export const /** Reports the Unicode-aware source position. */ unicodeOwned = 1;
