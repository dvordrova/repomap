import * as facade from "./star-index";

export function render(day: number): string {
  return facade.toText(day) + facade.getIndex(day);
}
