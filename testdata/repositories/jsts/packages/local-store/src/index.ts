// This repository package stores keys; it performs no HTTP requests.
export function get(key: string): string { return key }
export function createClient() { return { get } }
