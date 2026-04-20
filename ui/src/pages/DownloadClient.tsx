export default function DownloadClient() {
  return (
    <div className="max-w-2xl mx-auto py-12">
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-8 text-center">
        {/* Icon */}
        <div className="w-16 h-16 mx-auto mb-6 rounded-2xl bg-nvr-accent/20 flex items-center justify-center">
          <svg className="w-8 h-8 text-nvr-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 18h.01M8 21h8a2 2 0 002-2V5a2 2 0 00-2-2H8a2 2 0 00-2 2v14a2 2 0 002 2z" />
          </svg>
        </div>

        <h1 className="text-2xl font-bold text-nvr-text-primary mb-3">
          Download the Raikada Client
        </h1>
        <p className="text-nvr-text-secondary mb-8 leading-relaxed">
          Live view, playback, clip search, and recordings are available in the
          Raikada client app. This web console is for system administration only.
        </p>

        {/* Download options */}
        <div className="grid gap-4 sm:grid-cols-2 mb-8">
          <a
            href="https://github.com/EthanFlower1/mediamtx/releases/latest"
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-3 px-5 py-4 bg-nvr-bg-tertiary border border-nvr-border rounded-lg hover:border-nvr-accent/50 transition-colors group"
          >
            <svg className="w-6 h-6 text-nvr-text-muted group-hover:text-nvr-accent transition-colors" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
            </svg>
            <div className="text-left">
              <p className="text-sm font-semibold text-nvr-text-primary">Desktop</p>
              <p className="text-xs text-nvr-text-muted">Windows, macOS, Linux</p>
            </div>
          </a>
          <a
            href="https://github.com/EthanFlower1/mediamtx/releases/latest"
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-3 px-5 py-4 bg-nvr-bg-tertiary border border-nvr-border rounded-lg hover:border-nvr-accent/50 transition-colors group"
          >
            <svg className="w-6 h-6 text-nvr-text-muted group-hover:text-nvr-accent transition-colors" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 18h.01M8 21h8a2 2 0 002-2V5a2 2 0 00-2-2H8a2 2 0 00-2 2v14a2 2 0 002 2z" />
            </svg>
            <div className="text-left">
              <p className="text-sm font-semibold text-nvr-text-primary">Mobile</p>
              <p className="text-xs text-nvr-text-muted">iOS, Android</p>
            </div>
          </a>
        </div>

        {/* Setup instructions */}
        <div className="bg-nvr-bg-primary border border-nvr-border rounded-lg p-5 text-left">
          <h2 className="text-sm font-semibold text-nvr-text-primary mb-3">Quick Setup</h2>
          <ol className="text-sm text-nvr-text-secondary space-y-2 list-decimal list-inside">
            <li>Download and install the Raikada client for your platform</li>
            <li>Open the app and enter your server address</li>
            <li>Sign in with your NVR credentials</li>
            <li>Start viewing live feeds, playback, and clips</li>
          </ol>
        </div>
      </div>
    </div>
  )
}
