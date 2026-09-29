package storefixture

import "net/url"

// ReplicaOptions reads a replica URL's options the way litestream's S3
// client does, some of them in two spellings of one option:
//
//   - storageClass and storage-class: the else-if arm's header and body are
//     written again but for the option's word, so either spelling sets it;
//   - forcePathStyle and force-path-style: one || joins two reads compared
//     the same way, set under either spelling.
//
// The contrasts read several values: region once, endpoint and bucket in
// arms whose bodies store different names, and user and password joined
// by &&, both needed.
func ReplicaOptions(u *url.URL) ReplicaSettings {
	var settings ReplicaSettings
	query := u.Query()
	if v := query.Get("storageClass"); v != "" {
		settings.StorageClass = v
	} else if v := query.Get("storage-class"); v != "" {
		settings.StorageClass = v
	}
	settings.PathStyleSet = query.Get("forcePathStyle") != "" || query.Get("force-path-style") != ""
	settings.Region = query.Get("region")
	if v := query.Get("endpoint"); v != "" {
		settings.Endpoint = v
	} else if v := query.Get("bucket"); v != "" {
		settings.Bucket = v
	}
	settings.Credentials = query.Get("user") != "" && query.Get("password") != ""
	return settings
}

// ReplicaSettings are the options a replica URL sets.
type ReplicaSettings struct {
	StorageClass string
	PathStyleSet bool
	Region       string
	Endpoint     string
	Bucket       string
	Credentials  bool
}
