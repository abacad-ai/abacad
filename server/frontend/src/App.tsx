import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { AuthProvider, useAuth } from "@/auth";
import { Layout } from "@/components/Layout";
import { AuthPage } from "@/pages/AuthPage";
import { DevicesPage } from "@/pages/DevicesPage";
import { SettingsPage } from "@/pages/SettingsPage";

function Protected({ children }: { children: React.ReactNode }) {
  const { me, loading } = useAuth();
  if (loading) return <div className="p-10 text-center text-sm text-slate-500">Loading…</div>;
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
