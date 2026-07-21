using System.Security.Principal;

namespace Abacad;

// Windows has no per-capability TCC grant like macOS: a normal process can already
// read the UI Automation tree, capture the screen, and inject input. The one real
// gate is integrity level — input into windows owned by an *elevated* process is
// blocked by UIPI unless this app is elevated too. So "permissions" here is just a
// status the panel surfaces, not a set of prompts to walk the user through.
static class Permissions
{
    /// True when the app is running elevated (as administrator), which is required
    /// to drive elevated windows (UAC dialogs, admin apps, the secure desktop aside).
    public static bool IsElevated
    {
        get
        {
            try
            {
                using var id = WindowsIdentity.GetCurrent();
                return new WindowsPrincipal(id).IsInRole(WindowsBuiltInRole.Administrator);
            }
            catch { return false; }
        }
    }
}
