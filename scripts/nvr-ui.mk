.PHONY: nvr-ui
nvr-ui:
	cd ui && npm ci && npm run build
