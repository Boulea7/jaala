.PHONY: run build check test clean

# Serve the site at http://localhost:8085/jaala/. It builds once and has no file watcher, so restart
# it after an edit.
run:
	JAALA_DOCS_PORT=:8085 go run .

# Build the site into ./dist. s3gen writes pages only, so static/ is copied in after; without it the
# built site has no CSS or scripts. .nojekyll stops GitHub Pages from dropping files that start
# with an underscore.
build:
	go run . -build
	rm -rf dist/static
	cp -r static dist/static
	touch dist/.nojekyll

# The docsite's tests: templates parse, navigation is wired, includes resolve, and every internal
# link in the built site resolves (which needs the build first).
test: build
	go test -count=1 ./...

# The mission's exercise (#104): build the site and run every check on it.
check: test

clean:
	rm -rf dist
