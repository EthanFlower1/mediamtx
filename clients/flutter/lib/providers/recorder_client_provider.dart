// KAI-299 — Recorder endpoint resolution utility.
//
// In all-in-one mode, the Directory and Recorder share the same process and
// the same base URL, so `recorderEndpoint` is null in [Camera] and we fall
// back to the Directory's own URL. In multi-server mode, each Recorder
// registers its endpoint with the Directory, which echoes it in the camera
// list response as `recorder_endpoint`.

/// Resolves the correct Recorder API endpoint for a given camera.
///
/// In all-in-one mode, returns the same URL as the Directory
/// ([directoryEndpoint]). In multi-server mode, returns the Recorder's own
/// dedicated URL so the client sends data-plane requests (live view, playback,
/// export) directly to the owning Recorder instead of routing through the
/// Directory.
///
/// [recorderEndpoint] is sourced from [Camera.recorderEndpoint] — it is `null`
/// when the backend is running in all-in-one mode.
/// [directoryEndpoint] is the base URL of the Directory the client is currently
/// connected to (i.e. [HomeDirectoryConnection.endpointUrl]).
String resolveRecorderEndpoint(
  String? recorderEndpoint,
  String directoryEndpoint,
) {
  return recorderEndpoint ?? directoryEndpoint;
}
