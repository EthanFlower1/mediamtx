.PHONY: webui-embed
webui-embed:
	cd ui-v2 && npm ci && npm run build
	rm -rf internal/directory/webui/dist
	mkdir -p internal/directory/webui/dist
	cp -R ui-v2/dist/. internal/directory/webui/dist/
	@echo "webui embedded at internal/directory/webui/dist"
