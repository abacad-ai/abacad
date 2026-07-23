using FlaUI.UIA3;

namespace Abacad;

// Shared UI-Automation *client* (FlaUI / UIA3). Replaces System.Windows.Automation
// now that the app is WinUI 3 (no UseWPF) — same capability (read/drive other
// apps' UI trees), sourced from FlaUI instead of the WPF desktop assemblies. One
// instance is reused by UiTree (tree capture) and CommandDispatcher (input_text
// set-value).
static class Uia
{
    public static readonly UIA3Automation Automation = new();
}
