// A setting read as a configuration helper often reads it: from the
// environment, else a word kept for two of its keys only. fetchData's
// address is neither word; fetchStatic's is the URL.
export function settingOrDefault(key: string): string {
  const value = process.env[key]
  if (value) return value
  if (key === "staticBaseUrl") {
    return "https://cdn.example/static"
  } else if (key === "logConfig") {
    return "logs/app.log"
  }
  return ""
}

export function fetchData(): Promise<Response> {
  return fetch(settingOrDefault("dataSourceName"))
}

export function fetchStatic(): Promise<Response> {
  return fetch(settingOrDefault("staticBaseUrl"))
}
