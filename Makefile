

all: build

build:
	go install migrate

up: build
	./bin/migrate up

down: build
	./bin/migrate down