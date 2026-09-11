import { useCallback, useEffect, useMemo, useState } from "react";
import { CirclePlay, LoaderCircle, Pause, Play, SkipBack, SkipForward } from "lucide-react";
import { api, type DeviceView, type ReplaySession, type ReplayStep } from "@/lib/api";
import { clockTime, cn } from "@/lib/utils";
import { DeviceFrame } from "@/components/DeviceScreen";
import { type FormFactor } from "@/lib/devices";
import { Button } from "@/components/ui/button";

// ReplayPlayer watches back what an agent did on a device: the frames it saw, in
// order, with each action marked on the frame it acted on.
//
// Playback advances by STEP, not by wall clock. An agent session is mostly
// waiting — a screenshot, thirty seconds of the model thinking, a tap — so
// replaying it in real time would be minutes of a frozen picture. Each step
// holds for a fixed beat and its real elapsed time is printed instead, which is
// the information the wall clock was carrying anyway.
//
// The player is driven entirely by the recorded steps; nothing here talks to the
// device. A device that is offline, deleted from your desk, or on the other side
// of the world replays identically.

// stepHoldMs is one step's beat at 1×. Fast enough to feel like playback, slow
// enough to read the method label under the frame.
const STEP_HOLD_MS = 900;

const SPEEDS = [1, 2, 4] as const;

export function ReplayPlayer({ device, factor }: { device: DeviceView; factor: FormFactor }) {
  const [sessions, setSessions] = useState<ReplaySession[] | null>(null);
  const [recording, setRecording] = useState(device.replay);
  const [selected, setSelected] = useState<ReplaySession | null>(null);
  const [steps, setSteps] = useState<ReplayStep[] | null>(null);
  const [index, setIndex] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState<(typeof SPEEDS)[number]>(1);
  const [error, setError] = useState<string | null>(null);
  const [aspect, setAspect] = useState<number | null>(null);

  // Switching devices keeps this component mounted (same route, new param), so
  // the selection has to be dropped explicitly — otherwise the picker would keep
  // pointing at a session that belongs to the device you just navigated away
  // from. Declared before the fetch below so it runs first.
  useEffect(() => {
    setSelected(null);
  }, [device.id]);

  // Reload the session list whenever recording is toggled or the device changes:
  // a session that was still being written when the page loaded has grown since.
  useEffect(() => {
    let alive = true;
    setSessions(null);
    api
      .replaySessions(device.id)
      .then((res) => {
        if (!alive) return;
        setSessions(res.sessions);
        setRecording(res.recording);
        setSelected((prev) => prev ?? res.sessions[0] ?? null);
        setError(null);
      })
      .catch((err) => alive && setError((err as Error).message));
    return () => {
      alive = false;
    };
  }, [device.id, device.replay]);

  // Load the selected session's steps.
  useEffect(() => {
    if (!selected) {
      setSteps(null);
      return;
    }
    let alive = true;
    setSteps(null);
    setIndex(0);
    setPlaying(false);
    api
      .replaySteps(device.id, selected.start_ts, selected.end_ts)
      .then((res) => {
        if (!alive) return;
        setSteps(res.steps);
        setError(null);
      })
      .catch((err) => alive && setError((err as Error).message));
    return () => {
      alive = false;
    };
  }, [device.id, selected]);

  const total = steps?.length ?? 0;

  // Advance while playing, stopping on the last step rather than looping — a
  // session that restarts on its own makes it impossible to tell where it ended.
  useEffect(() => {
    if (!playing || total === 0) return;
    if (index >= total - 1) {
      setPlaying(false);
      return;
    }
    const timer = setTimeout(() => setIndex((i) => Math.min(i + 1, total - 1)), STEP_HOLD_MS / speed);
    return () => clearTimeout(timer);
  }, [playing, index, total, speed]);

  const step = useCallback(
    (delta: number) => {
      setPlaying(false);
      setIndex((i) => Math.max(0, Math.min(i + delta, total - 1)));
    },
    [total],
  );

  // Arrow keys step, space plays/pauses, Home/End jump — but only while the
  // player has focus, so they don't fight the rest of the page.
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (total === 0) return;
    switch (e.key) {
      case "ArrowLeft":
        e.preventDefault();
        step(-1);
        break;
      case "ArrowRight":
        e.preventDefault();
        step(1);
        break;
      case "Home":
        e.preventDefault();
        setPlaying(false);
        setIndex(0);
        break;
      case "End":
        e.preventDefault();
        setPlaying(false);
        setIndex(total - 1);
        break;
      case " ":
        e.preventDefault();
        setPlaying((p) => !p);
        break;
    }
  };

  // The frame to show is the most recent one AT OR BEFORE the current step: a
  // tap returns no image, so it is drawn over the screen it acted on rather than
  // blanking the player. This is why the step list and the frame track are the
  // same list — the gap is the information.
  const frame = useMemo(() => {
    if (!steps) return null;
    for (let i = Math.min(index, steps.length - 1); i >= 0; i--) {
      if (steps[i].frame_url) return steps[i];
    }
    return null;
  }, [steps, index]);

  const current = steps?.[index];

  if (error) {
    return <p className="py-6 text-center text-sm text-danger">{error}</p>;
  }
  if (sessions === null) {
    return <div className="skeleton h-56 rounded-md" aria-label="Loading replay" />;
  }
  if (sessions.length === 0) {
    return <EmptyState recording={recording} />;
  }

  return (
    // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
    <div tabIndex={0} onKeyDown={onKeyDown} className="rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand">
      <div className="flex flex-wrap items-center gap-2">
        <label htmlFor="replay-session" className="font-mono text-[11px] uppercase tracking-wider text-ink-subtle">
          Session
        </label>
        <select
          id="replay-session"
          className="h-10 min-w-0 flex-1 rounded-md border border-border bg-surface px-3 text-[13px] font-medium text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
          value={selected ? String(selected.start_ts) : ""}
          onChange={(e) => setSelected(sessions.find((s) => String(s.start_ts) === e.target.value) ?? null)}
        >
          {sessions.map((s) => (
            <option key={s.start_ts} value={s.start_ts}>
              {sessionLabel(s)}
            </option>
          ))}
        </select>
        {recording && (
          <span className="flex items-center gap-1.5 rounded-full bg-danger-soft px-2.5 py-1 font-mono text-[10px] font-bold uppercase tracking-wider text-danger">
            <span className="h-1.5 w-1.5 rounded-full bg-danger" aria-hidden />
            Recording
          </span>
        )}
      </div>

      <div className="mt-4">
        {steps === null ? (
          <div className="skeleton h-56 rounded-md" aria-label="Loading session" />
        ) : steps.length === 0 ? (
          <p className="py-6 text-center text-sm text-ink-muted">This session has no recorded steps.</p>
        ) : (
          <>
            <DeviceFrame factor={factor} aspect={aspect} maxWidth={factor === "handset" ? "max-w-[280px]" : ""} bare={!!frame}>
              {frame?.frame_url ? (
                <>
                  <img
                    src={frame.frame_url}
                    alt={`Recorded screen at ${clockTime(frame.ts)}`}
                    onLoad={(e) => {
                      const img = e.currentTarget;
                      if (img.naturalWidth && img.naturalHeight) setAspect(img.naturalWidth / img.naturalHeight);
                    }}
                    className="absolute inset-0 h-full w-full object-contain"
                  />
                  <Marker step={current} frame={frame} />
                </>
              ) : (
                <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-ink-subtle">
                  <CirclePlay size={20} strokeWidth={1.25} />
                  <span className="font-mono text-[10px] uppercase tracking-wider">No frame yet</span>
                </div>
              )}
            </DeviceFrame>

            <Scrubber steps={steps} index={index} onSeek={(i) => (setPlaying(false), setIndex(i))} />

            <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
              <div className="flex items-center gap-1">
                <Button variant="ghost" size="icon" aria-label="Previous step" disabled={index === 0} onClick={() => step(-1)}>
                  <SkipBack size={16} />
                </Button>
                <Button
                  variant="outline"
                  size="icon"
                  aria-label={playing ? "Pause" : "Play"}
                  onClick={() => {
                    // Replaying from the end should start over, not sit there.
                    if (index >= steps.length - 1) setIndex(0);
                    setPlaying((p) => !p);
                  }}
                >
                  {playing ? <Pause size={16} /> : <Play size={16} />}
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Next step"
                  disabled={index >= steps.length - 1}
                  onClick={() => step(1)}
                >
                  <SkipForward size={16} />
                </Button>
                <div role="group" aria-label="Playback speed" className="ml-2 flex items-center gap-1">
                  {SPEEDS.map((s) => (
                    <button
                      key={s}
                      type="button"
                      onClick={() => setSpeed(s)}
                      aria-pressed={speed === s}
                      className={cn(
                        "h-8 rounded-full px-2.5 font-mono text-[11px] font-bold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand",
                        speed === s ? "bg-brand-soft text-brand" : "text-ink-muted hover:text-ink",
                      )}
                    >
                      {s}×
                    </button>
                  ))}
                </div>
              </div>
              <p className="font-mono text-[11px] text-ink-subtle">
                {index + 1} / {steps.length}
              </p>
            </div>

            {current && <StepDetail step={current} previous={steps[index - 1]} />}
          </>
        )}
      </div>
    </div>
  );
}

// The scrubber doubles as the shape of the session: a tick per step, frames
// solid and frameless actions hollow, so the gaps between pictures are visible
// before you play it rather than discovered while scrubbing.
function Scrubber({ steps, index, onSeek }: { steps: ReplayStep[]; index: number; onSeek: (i: number) => void }) {
  return (
    <div className="mt-4">
      <input
        type="range"
        min={0}
        max={steps.length - 1}
        value={index}
        aria-label="Seek through the session"
        onChange={(e) => onSeek(Number(e.target.value))}
        className="w-full accent-brand"
      />
      {/* A tick per step: the agent's own frames solid, the ones replay captured
          after an action lighter, and frameless actions lightest. The shape of a
          session — where it looked and where it only acted — is legible before
          you play it. */}
      <div className="mt-1 flex gap-px" aria-hidden>
        {steps.map((s, i) => (
          <span
            key={s.id}
            className={cn(
              "h-1.5 flex-1 rounded-sm",
              i === index
                ? "bg-brand"
                : !s.frame_url
                  ? "bg-border"
                  : isAutoFrame(s)
                    ? "bg-border-strong/50"
                    : "bg-border-strong",
            )}
          />
        ))}
      </div>
    </div>
  );
}

// Marker draws where a pointer command landed, positioned as a percentage of the
// frame it is drawn over. A step with no marker (a screenshot, a keypress) draws
// nothing.
//
// The scale comes from the FRAME, not from the step carrying the marker. A tap
// returns no image, so its own w/h are zero — scaling by those silently drew
// nothing at all, which is the whole feature disappearing without an error. Both
// are the same screen in device pixels, so the frame's dimensions are the right
// basis; the image is rendered object-contain at its own aspect ratio, so
// percentages land correctly at any display size.
function Marker({ step, frame }: { step?: ReplayStep; frame?: ReplayStep | null }) {
  const m = step?.marker;
  if (!m || !frame?.w || !frame?.h) return null;
  const pct = (v: number, of: number) => `${Math.max(0, Math.min(100, (v / of) * 100))}%`;
  const end = m.x2 !== undefined && m.y2 !== undefined ? { left: pct(m.x2, frame.w), top: pct(m.y2, frame.h) } : null;
  return (
    <>
      <span
        className="pointer-events-none absolute h-5 w-5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-brand bg-brand/25"
        style={{ left: pct(m.x, frame.w), top: pct(m.y, frame.h) }}
        aria-hidden
      />
      {end && (
        <span
          className="pointer-events-none absolute h-3 w-3 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-brand/70"
          style={end}
          aria-hidden
        />
      )}
    </>
  );
}

// A step the SERVER captured after an action, rather than a command the agent
// issued. Worth distinguishing everywhere it appears: without it the timeline
// reads as though the agent took twice as many screenshots as it did, which is a
// false picture of how the agent actually worked.
const isAutoFrame = (s?: ReplayStep) => s?.source === "replay";

// What the current step was, and how long the agent waited before it. The gap is
// shown rather than the absolute clock because playback is step-paced: it is the
// only place the real timing survives.
function StepDetail({ step, previous }: { step: ReplayStep; previous?: ReplayStep }) {
  const gap = previous ? step.ts - previous.ts : 0;
  const auto = isAutoFrame(step);
  return (
    <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-border pt-3">
      <span className={cn("font-mono text-[13px] font-bold", auto ? "text-ink-muted" : "text-ink")}>
        {auto ? "screen" : step.method}
      </span>
      {auto && (
        <span className="rounded bg-surface-hover px-2 py-0.5 font-mono text-[10px] font-bold uppercase text-ink-subtle">
          after the action
        </span>
      )}
      {step.outcome && step.outcome !== "ok" && (
        <span className="rounded bg-danger-soft px-2 py-0.5 font-mono text-[10px] font-bold uppercase text-danger">
          {step.outcome}
        </span>
      )}
      <span className="font-mono text-[11px] text-ink-subtle">
        {clockTime(step.ts)}
        {step.duration_ms ? ` · ${step.duration_ms}ms` : ""}
        {gap > 0 ? ` · +${formatGap(gap)} since previous` : ""}
        {auto ? " · captured by replay" : step.actor_label ? ` · ${step.actor_label}` : ""}
      </span>
    </div>
  );
}

function EmptyState({ recording }: { recording: boolean }) {
  return (
    <div className="flex flex-col items-center gap-2 py-8 text-center">
      {recording ? <LoaderCircle size={18} className="animate-spin text-ink-subtle" /> : null}
      <p className="text-sm text-ink-muted">
        {recording
          ? "Recording is on. The next commands an agent runs on this device will appear here."
          : "Nothing recorded. Turn on Session replay under Setup to keep a frame-by-frame record of what agents do here."}
      </p>
    </div>
  );
}

// A session is identified by when it happened — there is no session id — so the
// label has to carry enough to tell two apart: the day and time it started, how
// long it ran, and how much is in it.
function sessionLabel(s: ReplaySession): string {
  const start = new Date(s.start_ts);
  const day = start.toLocaleDateString([], { month: "short", day: "numeric" });
  const ran = formatGap(s.end_ts - s.start_ts);
  const who = s.actor_label ? ` · ${s.actor_label}` : "";
  return `${day} ${clockTime(s.start_ts)} · ${s.steps} step${s.steps === 1 ? "" : "s"} over ${ran}${who}`;
}

function formatGap(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const secs = Math.round(ms / 1000);
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  const rem = secs % 60;
  return rem ? `${mins}m ${rem}s` : `${mins}m`;
}
