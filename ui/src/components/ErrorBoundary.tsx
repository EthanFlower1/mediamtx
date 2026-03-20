import React from 'react'

interface ErrorBoundaryState {
  hasError: boolean
  error: Error | null
}

interface ErrorBoundaryProps {
  children: React.ReactNode
}

export default class ErrorBoundary extends React.Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error }
  }

  handleRetry = () => {
    this.setState({ hasError: false, error: null })
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="min-h-screen bg-nvr-bg-primary flex items-center justify-center p-6">
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6 md:p-8 max-w-md w-full text-center">
            <div className="text-4xl mb-4 text-nvr-danger">!</div>
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-2">Something went wrong</h2>
            <p className="text-sm text-nvr-text-secondary mb-4">
              {this.state.error?.message || 'An unexpected error occurred.'}
            </p>
            <div className="flex gap-2 justify-center">
              <button
                onClick={this.handleRetry}
                className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm min-h-[44px]"
              >
                Retry
              </button>
              <button
                onClick={() => window.location.reload()}
                className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm min-h-[44px]"
              >
                Reload Page
              </button>
            </div>
          </div>
        </div>
      )
    }

    return this.props.children
  }
}
