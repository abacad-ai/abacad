import { BrowserRouter, Route, Routes } from "react-router-dom"

import { Toaster } from "@/components/ui/sonner"
import Dashboard from "@/pages/dashboard"
import NotFound from "@/pages/not-found"
import Viewer from "@/pages/viewer"

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/devices/:id" element={<Dashboard />} />
        <Route path="/skill" element={<Dashboard view="skill" />} />
        <Route path="/vnc/:id" element={<Viewer />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
      <Toaster position="bottom-right" />
    </BrowserRouter>
  )
}
