

all: build

build:
	go install migrate

up: build
	./bin/migrate up