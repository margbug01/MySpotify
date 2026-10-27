//go:build !darwin

package spotify

// mediaFormat is the files the web player asks for where Widevine
// decrypts: AAC encrypted with cenc.
const mediaFormat = "file_ids_mp4"

// adaptManifest leaves the manifest as the service writes it, for Widevine.
func adaptManifest(m, fileID string) string { return m }
