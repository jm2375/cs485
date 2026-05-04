import type { ReactNode } from 'react';
import { Navigate } from 'react-router-dom';
import { api } from '../api';

interface ProtectedRouteProps {
  children: ReactNode;
}

export default function ProtectedRoute({ children }: ProtectedRouteProps) {
  if (!api.isAuthenticated()) {
    return <Navigate to={`/login?return=${encodeURIComponent(location.pathname)}`} replace />;
  }
  return <>{children}</>;
}
