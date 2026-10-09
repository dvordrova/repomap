// The helper's one JSON value keeps native insertion/array order. Transport
// chunks follow the actual writable stream framing; they never limit evidence.
function* nativeJSONTokens(input, key = "", ancestors = new Set()) {
  let value = input
  if (value !== null && (typeof value === "object" || typeof value === "bigint") && typeof value.toJSON === "function") value = value.toJSON(key)
  if (value instanceof Number || value instanceof String || value instanceof Boolean) value = value.valueOf()
  if (value === null || typeof value !== "object") {
    try {
      const encoded = JSON.stringify(value)
      if (encoded !== undefined) yield encoded
    } catch (error) {
      throw new Error(`native JSON transport: indivisible value cannot be encoded: ${error.message}`, { cause: error })
    }
    return
  }
  if (ancestors.has(value)) throw new TypeError("native JSON transport: circular native value")
  ancestors.add(value)
  try {
    if (Array.isArray(value)) {
      yield "["
      for (let index = 0; index < value.length; index++) {
        if (index > 0) yield ","
        const tokens = nativeJSONTokens(value[index], String(index), ancestors)
        const first = tokens.next()
        if (first.done) yield "null"
        else { yield first.value; yield* tokens }
      }
      yield "]"
    } else {
      yield "{"
      let firstEntry = true
      for (const property of Object.keys(value)) {
        const tokens = nativeJSONTokens(value[property], property, ancestors)
        const first = tokens.next()
        if (first.done) continue
        if (!firstEntry) yield ","
        firstEntry = false
        yield JSON.stringify(property)
        yield ":"
        yield first.value
        yield* tokens
      }
      yield "}"
    }
  } finally {
    ancestors.delete(value)
  }
}

async function writeNativeJSON(value, stream) {
  const chunkUnits = stream.writableHighWaterMark
  if (!Number.isSafeInteger(chunkUnits) || chunkUnits < 1) throw new Error("native JSON transport: invalid writable stream framing")
  // The callback acknowledges each completed write, including a full pipe.
  // Keep an error listener while writing; the callback propagates its error.
  const onError = () => {}
  stream.on("error", onError)
  const write = (text) => new Promise((resolve, reject) => {
    stream.write(text, (error) => error ? reject(error) : resolve())
  })
  let pending = ""
  try {
    const tokens = nativeJSONTokens(value)
    const first = tokens.next()
    if (first.done) throw new Error("native JSON transport: root value has no JSON representation")
    function* completeTokens() { yield first.value; yield* tokens }
    for (const token of completeTokens()) {
      let position = 0
      while (position < token.length) {
        let end = Math.min(token.length, position + chunkUnits - pending.length)
        // Native stdout encodes each string write separately. Never split a
        // UTF16 surrogate pair into two replacement characters at a boundary.
        if (end < token.length && end > position && token.charCodeAt(end - 1) >= 0xd800 && token.charCodeAt(end - 1) <= 0xdbff && token.charCodeAt(end) >= 0xdc00 && token.charCodeAt(end) <= 0xdfff) end--
        if (end === position) {
          if (pending.length > 0) { await write(pending); pending = ""; continue }
          end = position + 2
        }
        pending += token.slice(position, end)
        position = end
        if (pending.length >= chunkUnits) { await write(pending); pending = "" }
      }
    }
    if (pending.length > 0) await write(pending)
  } finally {
    stream.off("error", onError)
  }
}
