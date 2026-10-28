//go:build !darwin

package spotify

// mediaFormat is the files the web player asks for where Widevine
// decrypts: AAC encrypted with cenc.
const mediaFormat = "file_ids_mp4"

// mediaFallbacks are the manifest formats asked for after mediaFormat, for
// a track that names none of its files in the first: the web player offers
// each track in more than one, and another rendition of the same audio
// plays where the first is missing (a relink, a region, an episode). The
// dual one is cenc too, which Widevine decrypts. It is a var for tests.
var mediaFallbacks = []string{"file_ids_mp4_dual"}

// adaptManifest leaves the manifest as the service writes it, for Widevine.
func adaptManifest(m, fileID string) string { return m }
