// The monorepo version, stamped here by `make bump-version`.
//
// Every other client reads the repo-root VERSION file at build time — Go through
// ldflags, the .app through an Info.plist substitution, Windows and Android
// through their build systems. SwiftPM has no equivalent hook for a plain
// executable target, and the CLI has no Info.plist to read, so the number is
// committed instead. Keep it in sync by using `make bump-version`; the CI release
// job fails the build if this number and VERSION disagree, which catches a stale one.
public enum AbacadVersion {
    public static let current = "0.5.4"
}
