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

    /// <summary>
    /// The full dial URL (wss://…?token=…). Written by `abacad connect` and by a
    /// human pasting into the window. Self-enrollment leaves this empty and uses
    /// RelayUrl + DeviceToken instead, so an existing install keeps behaving
    /// exactly as before.
    /// </summary>
    public static string ServerUrl
    {
        get => Read("server_url");
        set => Write("server_url", value);
    }

    // Self-enrollment credentials. The token is a bearer secret, so it gets the
    // same DPAPI treatment as the server URL rather than sitting in plaintext.
    public static string RelayUrl
    {
        get => Read("relay_url");
        set => Write("relay_url", value);
    }

    public static string DeviceId
    {
        get => Read("device_id");
        set => Write("device_id", value);
    }

    public static string DeviceToken
    {
        get => Read("device_token");
        set => Write("device_token", value);
    }

    /// <summary>
    /// Drop the self-enrollment credential, keeping the relay so a re-register
    /// goes back to the same server. Used when the relay stops recognizing us.
    /// </summary>
    public static void ClearEnrollment()
    {
        DeviceId = "";
        DeviceToken = "";
    }

    static string Read(string name)
    {
        try
        {
            var path = Path.Combine(Dir, name);
            if (!File.Exists(path)) return "";
            var dec = ProtectedData.Unprotect(
                File.ReadAllBytes(path), null, DataProtectionScope.CurrentUser);
            return Encoding.UTF8.GetString(dec);
        }
        catch { return ""; }
    }

    static void Write(string name, string? value)
    {
        try
        {
            Directory.CreateDirectory(Dir);
            var enc = ProtectedData.Protect(
                Encoding.UTF8.GetBytes(value ?? ""), null, DataProtectionScope.CurrentUser);
            File.WriteAllBytes(Path.Combine(Dir, name), enc);
        }
        catch { /* best effort; a missing store just means "reconnect next launch" */ }
    }
}
