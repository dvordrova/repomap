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

export function firstOf<T>(items: T[]): T {
  return items[0];
}

// TODO: document count validation.

// NOTE: count describes a quantity, not a list of levels.
