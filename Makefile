#
# Makefile
#

# ldflags variables to update --version
# short commit hash
COMMIT :=$(shell /usr/bin/git describe --always)
DATE :=$(shell /bin/date -u +"%Y-%m-%d-%H:%M")

all: local_builddocker_goigmpexample

local_builddocker_goigmpexample:
	docker buildx build --build-arg GOPRIVATE --file build/containers/goIGMPexample/Dockerfile . \
		--tag go-igmp-example:latest
	docker save go-igmp-example > ~/Downloads/go-igmp-example.tar
	scp ~/Downloads/go-igmp-example.tar dev-sen:
	ssh dev-sen docker load -i /home/das/go-igmp-example.tar
	ssh dev-sen docker compose --env-file /tmp/siden/dave-dev-sen/env --file /tmp/siden/dave-dev-sen/docker-compose.igmp.yml up -d --remove-orphans

