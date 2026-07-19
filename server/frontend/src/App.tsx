import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { AuthProvider, useAuth } from "@/auth";
import { Layout } from "@/components/Layout";
import { ActivitiesPage } from "@/pages/ActivitiesPage";
import { AuthPage } from "@/pages/AuthPage";
import { DevicesPage } from "@/pages/DevicesPage";
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

export function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginRoute />} />
          <Route
            path="/"
            element={
              <Protected>
                <DevicesPage />
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
