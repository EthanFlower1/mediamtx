export default function DownloadClient() {
  return (
    <div className="max-w-2xl mx-auto py-12">
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-8 text-center">
        <div className="w-16 h-16 mx-auto mb-6 rounded-2xl bg-nvr-accent/20 flex items-center justify-center">
          <svg className="w-8 h-8 text-nvr-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
          </svg>
        </div>

        <h1 className="text-2xl font-bold text-nvr-text-primary mb-3">
          Download the MediaMTX NVR Client
        </h1>

        <p className="text-nvr-text-secondary mb-8 leading-relaxed">
          Live view, playback, clip search, and recordings are available in the
          MediaMTX NVR client app. This web console is for system administration
          only (cameras, storage, users, and system settings).
        </p>

        <div className="grid gap-4 sm:grid-cols-2 mb-8">
          <a
            href="https://apps.apple.com"
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center justify-center gap-3 bg-nvr-bg-tertiary hover:bg-nvr-bg-tertiary/80 border border-nvr-border rounded-lg px-5 py-4 transition-colors group"
          >
            <svg className="w-8 h-8 text-nvr-text-primary" viewBox="0 0 24 24" fill="currentColor">
              <path d="M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.8-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z" />
            </svg>
            <div className="text-left">
              <div className="text-xs text-nvr-text-muted">Download on the</div>
              <div className="text-sm font-semibold text-nvr-text-primary group-hover:text-white transition-colors">App Store</div>
            </div>
          </a>

          <a
            href="https://play.google.com"
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center justify-center gap-3 bg-nvr-bg-tertiary hover:bg-nvr-bg-tertiary/80 border border-nvr-border rounded-lg px-5 py-4 transition-colors group"
          >
            <svg className="w-8 h-8 text-nvr-text-primary" viewBox="0 0 24 24" fill="currentColor">
              <path d="M3.609 1.814L13.792 12 3.61 22.186a.996.996 0 01-.61-.92V2.734a1 1 0 01.609-.92zm10.89 10.893l2.302 2.302-10.937 6.333 8.635-8.635zm3.199-3.199l2.302 2.302-2.302 2.302-2.698-2.302 2.698-2.302zM5.864 2.658L16.8 8.991l-2.302 2.302-8.634-8.635z" />
            </svg>
            <div className="text-left">
              <div className="text-xs text-nvr-text-muted">Get it on</div>
              <div className="text-sm font-semibold text-nvr-text-primary group-hover:text-white transition-colors">Google Play</div>
            </div>
          </a>
        </div>

        <div className="bg-nvr-bg-primary border border-nvr-border rounded-lg p-5">
          <h2 className="text-sm font-semibold text-nvr-text-primary mb-2">Desktop App</h2>
          <p className="text-sm text-nvr-text-secondary mb-3">
            A desktop client is also available for Windows, macOS, and Linux.
          </p>
          <a
            href="https://github.com/EthanFlower1/mediamtx/releases"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-2 text-sm font-medium text-nvr-accent hover:text-nvr-accent/80 transition-colors"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
            </svg>
            Download from GitHub Releases
          </a>
        </div>
      </div>
    </div>
  )
}
