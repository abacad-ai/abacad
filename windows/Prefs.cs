using System.Security.Cryptography;
using System.Text;

namespace Abacad;

// Persisted settings. The server URL carries the device token (?token=…), so it
// is encrypted at rest with DPAPI (CurrentUser scope) rather than left as
// plaintext — the Windows analogue of the macOS client's Keychain storage. Only
// the same Windows user account on this machine can decrypt it.
static class Prefs
{
    static string Dir => Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "abacad");
    static string FilePath => Path.Combine(Dir, "server_url");

    public static string ServerUrl
    {
        get
        {
            try
            {
                if (!File.Exists(FilePath)) return "";
                var enc = File.ReadAllBytes(FilePath);
                var dec = ProtectedData.Unprotect(enc, null, DataProtectionScope.CurrentUser);
                return Encoding.UTF8.GetString(dec);
            }
            catch { return ""; }
        }
        set
        {
            try
            {
                Directory.CreateDirectory(Dir);
                var enc = ProtectedData.Protect(
                    Encoding.UTF8.GetBytes(value ?? ""), null, DataProtectionScope.CurrentUser);
                File.WriteAllBytes(FilePath, enc);
            }
            catch { /* best effort; a missing store just means "reconnect next launch" */ }
        }
    }
}
