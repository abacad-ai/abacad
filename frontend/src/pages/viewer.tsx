import { ArrowLeft } from "lucide-react"
import { useEffect, useState } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import { VncStage } from "@/components/vnc-stage"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { ApiError, api } from "@/lib/api"
import type { Device } from "@/lib/types"

// Standalone screen: the console already shows the live screen inline, this
// page exists for a window of its own.
export default function Viewer() {
  const { id = "" } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const [device, setDevice] = useState<Device | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState("")

  useEffect(() => {
    let cancelled = false
    // Switching devices reuses the page, so the previous result has to be
    // cleared before the new fetch starts.
    // oxlint-disable-next-line react/set-state-in-effect
    setLoading(true)
    setLoadError("")
    api
      .devices()
      .then((response) => {
        if (cancelled) return
        const found = response.devices.find((candidate) => candidate.id === id) ?? null
        if (!found) {
          setLoadError("Device not found.")
          setLoading(false)
          return
        }
        if (found.mode === "direct") {
          setLoadError(
            `This device is catalog-only. Connect your VNC client directly to ${found.host}:${found.port}.`,
          )
          setLoading(false)
          return
        }
        setDevice(found)
        setLoading(false)
      })
      .catch((caught) => {
        if (cancelled) return
        if (caught instanceof ApiError && caught.status === 401) {
          navigate(`/?next=${encodeURIComponent(`/vnc/${id}`)}`, { replace: true })
          return
        }
        setLoadError(caught instanceof Error ? caught.message : "Could not load the device.")
        setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [id, navigate])

  if (loading) {
    return (
      <div className="grid min-h-screen place-items-center bg-neutral-950 text-sm text-neutral-300">
        Connecting…
      </div>
    )
  }

  if (loadError || !device) {
    return (
      <div className="grid min-h-screen place-items-center p-6">
        <Card className="max-w-md items-center gap-3 p-6 text-center">
          <p className="text-base font-medium">Can’t open this device</p>
          <p className="text-sm text-muted-foreground">{loadError}</p>
          <Button asChild variant="outline" className="mt-2">
            <Link to="/">
              <ArrowLeft />
              Back to devices
            </Link>
          </Button>
        </Card>
      </div>
    )
  }

  return (
    <div className="h-screen">
      <VncStage
        device={device}
        leading={
          <Button
            asChild
            size="sm"
            variant="ghost"
            className="ml-0.5 mr-1 text-neutral-400 hover:bg-white/10 hover:text-white"
          >
            <Link to={`/devices/${encodeURIComponent(id)}`}>
              <ArrowLeft />
              Console
            </Link>
          </Button>
        }
      />
    </div>
  )
}
