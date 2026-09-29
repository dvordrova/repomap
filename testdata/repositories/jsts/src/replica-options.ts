// A replica URL's options, read the way litestream's S3 client reads them,
// some in two spellings of one option:
//
//   - storageClass and storage-class: `??` takes the second spelling when the
//     first is absent, one value;
//   - forcePathStyle and force-path-style: one || joins two reads compared
//     the same way, set under either spelling;
//   - verbose and v: the else-if arm's condition and body are written again
//     but for the option's word.
//
// The contrasts read several values: region once, endpoint and bucket in
// arms whose bodies set different things, and user and password joined by
// &&, both needed.
export function replicaOptions(raw: string) {
  const query = new URL(raw).searchParams
  const storageClass = query.get("storageClass") ?? query.get("storage-class")
  const pathStyleSet = query.get("forcePathStyle") !== null || query.get("force-path-style") !== null
  const region = query.get("region")
  let verbose = false
  if (query.has("verbose")) {
    verbose = true
  } else if (query.has("v")) {
    verbose = true
  }
  let target = ""
  if (query.has("endpoint")) {
    target = "endpoint"
  } else if (query.has("bucket")) {
    target = "bucket"
  }
  const credentials = query.get("user") !== null && query.get("password") !== null
  return { storageClass, pathStyleSet, region, verbose, target, credentials }
}
