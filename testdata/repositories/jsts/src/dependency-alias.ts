import { embed } from "@fixture/dependency-alias";
// The alias is configured, but its dependency is not installed in this fixture.
export function embedMissingSDK() { return embed(); }
