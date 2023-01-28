# What is this?
A kubewatch alternative that supports CRDs. The plan is to add this functionality into kubewatch itself eventually.
It's only standlone as a POC.

# Instructions
There are no prebuilt binaries yet, so you'll have to compile before running. Fortunately, that's simple.

1. Compile with `go build`
2. Run with `./robusta-kubewatch`

You can customize which resources to listen to (including CRDs) by modifying the source code [here](https://github.com/robusta-dev/robusta-topology-watcher/blob/master/main.go#L82).

# Status
- [x] Listen to CRDs
- [ ] Merge into kubewatch
