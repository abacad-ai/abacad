namespace Abacad;

// Executes an ordered `composite` step list on-device with real timing — the
// low-level primitive the named verbs are sugar over. Steps carry an "op":
//
//   pointer_down {x,y,button?}   pointer_move {x,y}   pointer_up {button?}
//   key_down {key}   key_up {key}   type {text}
//   click {x,y,button?,count?,modifiers?}   wait {ms}   screenshot {}
//
// Any screenshot steps return their frames in order under {"shots": [...]}.
// Single-pointer only (mirrors macos/Composite.swift).
static class Composite
{
    public static async Task<Dictionary<string, object?>> Run(List<Dictionary<string, object?>> steps)
    {
        var shots = new List<object?>();
        int lastX = 0, lastY = 0;

        foreach (var step in steps)
        {
            switch (step.Str("op"))
            {
                case "pointer_down":
                    lastX = step.Int("x"); lastY = step.Int("y");
                    InputInjection.PointerDown(lastX, lastY, step.Str("button", "left"));
                    break;
                case "pointer_move":
                    lastX = step.Int("x"); lastY = step.Int("y");
                    InputInjection.PointerMove(lastX, lastY);
                    break;
                case "pointer_up":
                    InputInjection.PointerUp(lastX, lastY, step.Str("button", "left"));
                    break;
                case "key_down":
                    InputInjection.KeyByName(step.Str("key"), down: true);
                    break;
                case "key_up":
                    InputInjection.KeyByName(step.Str("key"), down: false);
                    break;
                case "type":
                    InputInjection.TypeText(step.Str("text"));
                    break;
                case "click":
                    InputInjection.Click(step.Int("x"), step.Int("y"),
                        step.Str("button", "left"), step.Int("count", 1), step.Strings("modifiers"));
                    break;
                case "wait":
                    int ms = step.Int("ms");
                    if (ms > 0) await Task.Delay(ms);
                    break;
                case "screenshot":
                    var shot = await Task.Run(() => ScreenCapture.Capture());
                    shots.Add(new Dictionary<string, object?>
                    {
                        ["w"] = shot.W,
                        ["h"] = shot.H,
                        ["png_base64"] = shot.Base64,
                    });
                    break;
                default:
                    throw new CmdException($"composite: unknown op \"{step.Str("op")}\"");
            }
        }

        return new Dictionary<string, object?> { ["shots"] = shots };
    }
}
