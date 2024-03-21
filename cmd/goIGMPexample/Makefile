#
# Makefile
#

# ldflags variables to update --version
# short commit hash
COMMIT :=$(shell /usr/bin/git describe --always)
DATE :=$(shell /bin/date -u +"%Y-%m-%d-%H:%M")
BINARY := goIGMPexample

all: clean build version

test:
	go test

clean:
	[ -f ${BINARY} ] && /bin/rm -rf ./${BINARY} || true

build:
	CGO_ENABLED=0 go build -ldflags "-X main.commit=${COMMIT} -X main.date=${DATE}" -o ./${BINARY} ./${BINARY}.go

br: build ./${BINARY}

br: build rsync

rsync:
	rsync -av /home/das/Downloads/goIGMP/ dev-sen:./goIGMP/

bsc: build scp

scp:
	scp ./${BINARY} dev-sen:
	#ssh dev-sen ./${BINARY}

sshfs_cp_and_run:
	cp ./${BINARY} ~/Downloads/vsen/
	ssh dev-sen ./mounted/goIGMPexample

# https://words.filippo.io/shrink-your-go-binaries-with-this-one-weird-trick/
buildsmall:
	CGO_ENABLED=0 go build -ldflags "-s -w -X main.commit=${COMMIT} -X main.date=${DATE}" -o ./${BINARY} ./${BINARY}.go

shrink:
	upx --brute ./${BINARY}

version:
	./${BINARY} --version

mount:
	sshfs dev-sen:/home/das/mounted ~/Downloads/vsen/

proxyTest:
	./${BINARY} \
		-outName enp1s0 \
		-altName gre0 \
		-inName br-siden \
		-proxyOutIn \
		-unicastProxyInToOut
#ip link add gre0 type gre

makeGREint:
	ip link add gre0 type gre
	ip addr add 10.0.0.1/32 dev gre0

uniTest:
	./${BINARY} \
		-promListen :7600 \
		-outName br-siden \
		-queryNotify \
		-membershipReportsFromNetwork \
		-membershipReportsToNetwork \
		-unicastMembershipReports \
		-connectQueryToReport
# --proxyOutIn false \
# --proxyInOut false \

#
