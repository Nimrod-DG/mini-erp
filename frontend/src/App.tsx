import { Navigate, BrowserRouter, Route, Routes } from "react-router-dom";
import type { ReactNode } from "react";

import { ProtectedRoute } from "./components/ProtectedRoute";
import { ToastProvider } from "./components/Toasts";
import {
  Home,
  RequireModule,
  RequireSuperadmin,
  RequireTenantAdmin,
} from "./components/RequireRole";
import { AuthProvider, useAuth } from "./hooks/useAuth";
import { ThemeProvider } from "./hooks/useTheme";
import { Dashboard } from "./pages/Dashboard";
import { Login } from "./pages/Login";
import { ResetPassword } from "./pages/ResetPassword";
import { TenantDetailPage } from "./pages/admin/TenantDetail";
import { TenantList } from "./pages/admin/TenantList";
import { TenantNew } from "./pages/admin/TenantNew";
import { LedgerPage } from "./pages/inventory/LedgerPage";
import { ProductDetailPage } from "./pages/inventory/ProductDetail";
import { ProductList } from "./pages/inventory/ProductList";
import { ProductNew } from "./pages/inventory/ProductNew";
import { StockGrid } from "./pages/inventory/StockGrid";
import { WarehouseList } from "./pages/inventory/WarehouseList";
import { OrderDetailPage } from "./pages/procurement/OrderDetail";
import { OrderList } from "./pages/procurement/OrderList";
import { RequisitionDetailPage } from "./pages/procurement/RequisitionDetail";
import { RequisitionList } from "./pages/procurement/RequisitionList";
import { RequisitionNew } from "./pages/procurement/RequisitionNew";
import { SupplierList } from "./pages/procurement/SupplierList";
import { UserDetailPage } from "./pages/settings/UserDetail";
import { UserList } from "./pages/settings/UserList";
import { UserNew } from "./pages/settings/UserNew";

/** /login, but only while signed out. Landing on it with a live session would
 *  otherwise offer a form that immediately bounces you back. */
function LoginRoute() {
  const auth = useAuth();
  if (auth.status === "signedIn") return <Navigate to="/" replace />;
  return <Login />;
}

/** Every signed-in screen sits behind ProtectedRoute; the plane guards nest
 *  inside it, because they read the resolved identity it waits for. */
function Signed({ children }: { children: ReactNode }) {
  return <ProtectedRoute>{children}</ProtectedRoute>;
}

export default function App() {
  return (
    <ThemeProvider>
      {/* Outside BrowserRouter, so a toast raised by an action that then
          navigates — "product added", say — survives the navigation. */}
      <ToastProvider>
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
                  <Signed>
                    <Home>
                      <Dashboard />
                    </Home>
                  </Signed>
                }
              />

              {/* Platform plane — superadmin (§5.7). */}
              <Route
                path="/admin/tenants"
                element={
                  <Signed>
                    <RequireSuperadmin>
                      <TenantList />
                    </RequireSuperadmin>
                  </Signed>
                }
              />
              <Route
                path="/admin/tenants/new"
                element={
                  <Signed>
                    <RequireSuperadmin>
                      <TenantNew />
                    </RequireSuperadmin>
                  </Signed>
                }
              />
              <Route
                path="/admin/tenants/:id"
                element={
                  <Signed>
                    <RequireSuperadmin>
                      <TenantDetailPage />
                    </RequireSuperadmin>
                  </Signed>
                }
              />

              {/* Tenant plane — workspace admin (§5.7). */}
              <Route
                path="/settings/users"
                element={
                  <Signed>
                    <RequireTenantAdmin>
                      <UserList />
                    </RequireTenantAdmin>
                  </Signed>
                }
              />
              {/* Before the :id route, or "new" is read as a user ID. */}
              <Route
                path="/settings/users/new"
                element={
                  <Signed>
                    <RequireTenantAdmin>
                      <UserNew />
                    </RequireTenantAdmin>
                  </Signed>
                }
              />
              <Route
                path="/settings/users/:id"
                element={
                  <Signed>
                    <RequireTenantAdmin>
                      <UserDetailPage />
                    </RequireTenantAdmin>
                  </Signed>
                }
              />

              {/* Procurement (§10.3). Same shape as inventory below: the guard
                is cosmetic, and every route is independently gated server-side
                at the levels in §9.4 (I12). */}
              <Route
                path="/procurement/requisitions"
                element={
                  <Signed>
                    <RequireModule module="procurement">
                      <RequisitionList />
                    </RequireModule>
                  </Signed>
                }
              />
              {/* Before the :id route, or "new" is read as a requisition ID. */}
              <Route
                path="/procurement/requisitions/new"
                element={
                  <Signed>
                    <RequireModule module="procurement">
                      <RequisitionNew />
                    </RequireModule>
                  </Signed>
                }
              />
              <Route
                path="/procurement/requisitions/:id"
                element={
                  <Signed>
                    <RequireModule module="procurement">
                      <RequisitionDetailPage />
                    </RequireModule>
                  </Signed>
                }
              />
              <Route
                path="/procurement/orders"
                element={
                  <Signed>
                    <RequireModule module="procurement">
                      <OrderList />
                    </RequireModule>
                  </Signed>
                }
              />
              <Route
                path="/procurement/orders/:id"
                element={
                  <Signed>
                    <RequireModule module="procurement">
                      <OrderDetailPage />
                    </RequireModule>
                  </Signed>
                }
              />
              <Route
                path="/procurement/suppliers"
                element={
                  <Signed>
                    <RequireModule module="procurement">
                      <SupplierList />
                    </RequireModule>
                  </Signed>
                }
              />

              {/* Inventory (§10.4). RequireModule hides the whole module from
                someone who holds nothing in it — cosmetically. Every route
                below is independently gated server-side by the real
                RequireModule, at the levels in §9.5 (I12). */}
              <Route
                path="/inventory/products"
                element={
                  <Signed>
                    <RequireModule module="inventory">
                      <ProductList />
                    </RequireModule>
                  </Signed>
                }
              />
              {/* Before the :id route, or "new" is read as a product ID. */}
              <Route
                path="/inventory/products/new"
                element={
                  <Signed>
                    <RequireModule module="inventory">
                      <ProductNew />
                    </RequireModule>
                  </Signed>
                }
              />
              <Route
                path="/inventory/products/:id"
                element={
                  <Signed>
                    <RequireModule module="inventory">
                      <ProductDetailPage />
                    </RequireModule>
                  </Signed>
                }
              />
              <Route
                path="/inventory/warehouses"
                element={
                  <Signed>
                    <RequireModule module="inventory">
                      <WarehouseList />
                    </RequireModule>
                  </Signed>
                }
              />
              <Route
                path="/inventory/stock"
                element={
                  <Signed>
                    <RequireModule module="inventory">
                      <StockGrid />
                    </RequireModule>
                  </Signed>
                }
              />
              <Route
                path="/inventory/ledger"
                element={
                  <Signed>
                    <RequireModule module="inventory">
                      <LedgerPage />
                    </RequireModule>
                  </Signed>
                }
              />

              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </AuthProvider>
        </BrowserRouter>
      </ToastProvider>
    </ThemeProvider>
  );
}
