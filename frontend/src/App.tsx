import { Navigate, BrowserRouter, Route, Routes } from "react-router-dom";

import { ProtectedRoute } from "./components/ProtectedRoute";
import { AuthProvider, useAuth } from "./hooks/useAuth";
import { ThemeProvider } from "./hooks/useTheme";
import { Dashboard } from "./pages/Dashboard";
import { Login } from "./pages/Login";
import { ResetPassword } from "./pages/ResetPassword";

/** /login, but only while signed out. Landing on it with a live session would
 *  otherwise offer a form that immediately bounces you back. */
function LoginRoute() {
  const auth = useAuth();
  if (auth.status === "signedIn") return <Navigate to="/" replace />;
  return <Login />;
}

export default function App() {
  return (
    <ThemeProvider>
      <BrowserRouter>
        <AuthProvider>
          <Routes>
            <Route path="/login" element={<LoginRoute />} />
            {/* Public and outside ProtectedRoute: someone resetting a password
                is by definition signed out. This is the target of the Firebase
                Console's "Customize action URL". */}
            <Route path="/auth/action" element={<ResetPassword />} />
            <Route
              path="/"
              element={
                <ProtectedRoute>
                  <Dashboard />
                </ProtectedRoute>
              }
            />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </AuthProvider>
      </BrowserRouter>
    </ThemeProvider>
  );
}
