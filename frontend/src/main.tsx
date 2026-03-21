import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import 'leaflet/dist/leaflet.css'
import './index.css'
import App from './App.tsx'
import LoginPage from './pages/LoginPage.tsx'
import InviteAcceptPage from './pages/InviteAcceptPage.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<App />} />
        <Route path="/login" element={<LoginPage />} />
        <Route path="/invite/:inviteCode" element={<InviteAcceptPage mode="link" />} />
        <Route path="/accept/:token" element={<InviteAcceptPage mode="token" />} />
      </Routes>
    </BrowserRouter>
  </StrictMode>,
)
