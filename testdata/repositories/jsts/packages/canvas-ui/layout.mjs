export function layout(groups) {
  return groups.map((name, index) => ({ name, x: index * 200, y: 0 }))
}
