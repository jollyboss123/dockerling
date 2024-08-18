# dockerling

A naive implementation of docker using chroot for file system isolation and linux PID for process isolation.

### references
- [What is chroot Linux sys call and How to Control It: Sandbox with Seccomp](https://medium.com/@seifeddinerajhi/what-is-chroot-linux-sys-call-and-how-to-control-it-sandbox-with-seccomp-b9b9a2bedfa4)
- [Why Pivot Root is Used for Containers](https://tbhaxor.com/pivot-root-vs-chroot-for-containers/)
- [Why Go’s Cmd.Run fails without /dev/null](https://rohitpaulk.com/articles/cmd-run-dev-null)
- [Linux namespace in Go - Part 1, UTS and PID](https://songrgg.github.io/programming/linux-namespace-part01-uts-pid/)
- [pid_namespaces(7) — Linux manual page](https://man7.org/linux/man-pages/man7/pid_namespaces.7.html)
- [Pulling a layer](https://distribution.github.io/distribution/spec/api/#pulling-a-layer)