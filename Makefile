.PHONY: build frontend test run clean

build: frontend
	go build -buildvcs=false -o airquality .

frontend: frontend/node_modules
	npm --prefix frontend run build

frontend/node_modules: frontend/package.json
	npm --prefix frontend install
	@touch $@

test: frontend/node_modules
	npm --prefix frontend test
	go test ./...

run: build
	./airquality

clean:
	rm -f airquality rooms.html
	rm -rf frontend/dist frontend/dist-rooms
