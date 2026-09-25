import { useEffect, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { api, type User } from "./api";
import Login from "./pages/Login";
import Workspace from "./pages/Workspace";

export default function App() {
  const [user, setUser] = useState<User | null | undefined>(undefined);
  useEffect(() => {
    api.me().then(setUser).catch(() => setUser(null));
  }, []);
  if (user === undefined) return <div className="empty">Loading…</div>;
  return (
    <Routes>
      <Route path="/login" element={user ? <Navigate to="/cases" replace /> : <Login onSignedIn={setUser} />} />
      <Route path="/cases/:caseId?" element={user ? <Workspace user={user} onSignedOut={() => setUser(null)} /> : <Navigate to="/login" replace />} />
      <Route path="*" element={<Navigate to={user ? "/cases" : "/login"} replace />} />
    </Routes>
  );
}
