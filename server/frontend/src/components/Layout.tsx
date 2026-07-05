import { Link, useLocation, useNavigate } from "react-router-dom";
import { LogOut, Smartphone, Settings } from "lucide-react";
import { type ReactNode } from "react";
import { api } from "@/lib/api";
import { useAuth } from "@/auth";
import { cn } from "@/lib/utils";

export function Layout({ children }: { children: ReactNode }) {
  const { me, setMe } = useAuth();
  const nav = useNavigate();
  const loc = useLocation();

  const logout = async () => {
    await api.logout();
    setMe(null);
    nav("/login");
  };

  const NavLink = ({ to, icon, label }: { to: string; icon: ReactNode; label: string }) => (
    <Link
      to={to}
      className={cn(
        "flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
        loc.pathname === to ? "bg-slate-800 text-white" : "text-slate-400 hover:bg-slate-800/60 hover:text-slate-200",
      )}
    >
      {icon}
      {label}
    </Link>
  );

  return (
    <div className="mx-auto flex min-h-full max-w-3xl flex-col px-4">
      <header className="flex items-center justify-between py-5">
        <Link to="/" className="flex items-center gap-2.5">
          <img src="/icon.svg" alt="" className="h-8 w-8 rounded-lg bg-white p-0.5" />
          <span className="text-lg font-bold tracking-tight">Abacad</span>
        </Link>
        <nav className="flex items-center gap-1">
          <NavLink to="/" icon={<Smartphone size={16} />} label="Devices" />
          <NavLink to="/settings" icon={<Settings size={16} />} label="Settings" />
          {me && (
            <button
              onClick={logout}
              className="ml-1 flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-slate-400 hover:bg-slate-800/60 hover:text-slate-200"
              title={me.email}
            >
              <LogOut size={16} />
            </button>
          )}
        </nav>
      </header>
      <main className="flex-1 pb-16">{children}</main>
    </div>
  );
}
