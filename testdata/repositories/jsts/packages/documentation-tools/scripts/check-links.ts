export function checkLinks(link: string): boolean {
  return link.startsWith("https://")
}

checkLinks("https://example.com/guide")

for (const link of ["https://example.com/reference"]) {
  checkLinks(link)
}
