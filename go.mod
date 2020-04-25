module github.com/protosio/cli

go 1.14

replace github.com/protosio/protos => ../backend

require (
	cuelang.org/go v0.0.15
	github.com/AlecAivazis/survey/v2 v2.0.7
	github.com/Masterminds/semver v1.5.0
	github.com/Sereal/Sereal v0.0.0-20200326150110-2c0ed69a855f // indirect
	github.com/asdine/storm v2.1.2+incompatible
	github.com/bramvdbogaerde/go-scp v0.0.0-20200119201711-987556b8bdd7
	github.com/cpuguy83/go-md2man/v2 v2.0.0 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/mikesmitty/edkey v0.0.0-20170222072505-3356ea4e686a
	github.com/pkg/errors v0.9.1
	github.com/protosio/protos v0.0.0-20200408102450-95ad50dc9dc1
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.6
	github.com/sirupsen/logrus v1.5.0
	github.com/urfave/cli/v2 v2.2.0
	golang.org/x/crypto v0.0.0-20200406173513-056763e48d71
	google.golang.org/appengine v1.6.1 // indirect
)
