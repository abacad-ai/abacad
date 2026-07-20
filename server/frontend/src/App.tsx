import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { AuthProvider, useAuth } from "@/auth";
import { Layout } from "@/components/Layout";
import { ActivitiesPage } from "@/pages/ActivitiesPage";
import { AuthPage } from "@/pages/AuthPage";
import { DeviceDetailPage } from "@/pages/DeviceDetailPage";
import { DevicesPage } from "@/pages/DevicesPage";
import { DownloadsPage } from "@/pages/DownloadsPage";
import { LandingPage } from "@/pages/LandingPage";
import { AccessPage } from "@/pages/AccessPage";
import { SettingsPage } from "@/pages/SettingsPage";

function Protected({ children }: { children: React.ReactNode }) {
  const { me, loading } = useAuth();
  if (loading) {
    return (
      <div className="flex min-h-dvh items-center justify-center bg-canvas text-ink">
        <div className="flex items-center gap-3 text-sm text-ink-muted">
          <span className="h-2 w-2 animate-pulse rounded-full bg-brand" />
          Loading workspace
        </div>
      </div>
    );
  }
  if (!me) return <Navigate to="/login" replace />;
  return <Layout>{children}</Layout>;
}

function LoginRoute() {
  const { me, loading } = useAuth();
  if (loading) return null;
  if (me) return <Navigate to="/" replace />;
  return <AuthPage />;
}

// "/" is the devices console once signed in; anonymous visitors get the public
// landing page (which doubles as the homepage Google's OAuth review loads).
// While auth is still resolving, Protected renders the loading state.
function RootRoute() {
  const { me, loading } = useAuth();
  if (!loading && !me) return <LandingPage />;
  return (
    <Protected>
      <DevicesPage />
    </Protected>
  );
}

export function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<RootRoute />} />
          <Route path="/login" element={<LoginRoute />} />
          {/* Public: you download a client before you have an account. The
              artifacts themselves are served by Go at /downloads/<file>. */}
          <Route path="/downloads" element={<DownloadsPage />} />
          {/* Devices moved to "/"; keep the old path working for bookmarks. */}
          <Route path="/devices" element={<Navigate to="/" replace />} />
          <Route
            path="/devices/:id"
            element={
              <Protected>
                <DeviceDetailPage />
              </Protected>
            }
          />
          <Route
            path="/activities"
            element={
              <Protected>
                <ActivitiesPage />
              </Protected>
            }
          />
          <Route
            path="/access"
            element={
              <Protected>
                <AccessPage />
              </Protected>
            }
          />
          <Route
            path="/settings"
            element={
              <Protected>
                <SettingsPage />
              </Protected>
            }
          />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}
