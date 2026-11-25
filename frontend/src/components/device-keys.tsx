import { ArrowLeft, Home, Layers2, Power, Volume1, Volume2 } from "lucide-react"
import { Fragment, type RefObject } from "react"

import { ANDROID_KEYS, sendDeviceKey, type DeviceKey } from "@/lib/device-keys"

const ICONS: Record<string, typeof Power> = {
  back: ArrowLeft,
  home: Home,
  recents: Layers2,
  power: Power,
  "volume-down": Volume1,
  "volume-up": Volume2,
}

// The screen's own buttons, for devices that have them. They sit over the frame
// rather than inside it: the vendored noVNC app is upstream's tree, unpatched,
// and these chords are ours to keep.
export function DeviceKeys({ frame }: { frame: RefObject<HTMLIFrameElement | null> }) {
  return (
    <div className="absolute top-3 left-3 z-10 flex items-center overflow-hidden rounded-lg border border-white/12 bg-ink-raised/85 backdrop-blur-sm">
      {ANDROID_KEYS.map((action, index) => (
        <Fragment key={action.id}>
          {index === 3 && <span aria-hidden="true" className="h-4 w-px bg-white/10" />}
          <DeviceKeyButton
            action={action}
            onPress={() => sendDeviceKey(frame.current, action)}
          />
        </Fragment>
      ))}
    </div>
  )
}

function DeviceKeyButton({
  action,
  onPress,
}: {
  action: DeviceKey
  onPress: () => void
}) {
  const Icon = ICONS[action.id] ?? Power
  return (
    <button
      type="button"
      onClick={onPress}
      title={`${action.label} · ${action.chord}`}
      aria-label={`${action.label} (${action.chord})`}
      className="grid size-8 cursor-pointer place-items-center text-white/70 transition-colors hover:bg-white/10 hover:text-white focus-visible:bg-white/10 focus-visible:text-white focus-visible:outline-none"
    >
      <Icon className="size-3.5" />
    </button>
  )
}
