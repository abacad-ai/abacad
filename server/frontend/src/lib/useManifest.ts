import { useEffect, useState } from "react";
import { api, type Build } from "@/lib/api";

// Fetch the client downloads manifest once. `builds` is null while loading, then
// the current build per platform+arch. A missing manifest is not an error (a
// fresh server has nothing staged) — api.manifest() resolves it to an empty list,
// so consumers render "not available yet". `error` is set only on a real failure.
export function useManifest(): { builds: Build[] | null; error: string | null } {
  const [builds, setBuilds] = useState<Build[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    api
      .manifest()
      .then((m) => live && setBuilds(m.builds))
      .catch((err: Error) => live && setError(err.message));
    return () => {
      live = false;
    };
  }, []);

  return { builds, error };
}
