import { useState } from "react";
import AdministratorPage from "./pages/AdministratorPage";
import AuthPage from "./pages/AuthPage";
import ModeratorApp from "./pages/moderator/ModeratorApp";
import StudentFeedbackPage from "./pages/StudentFeedbackPage";
import { logout, type AuthSession } from "./services/auth";

function App() {
  const [session, setSession] = useState<AuthSession | null>(null);
  const [sessionError, setSessionError] = useState("");

  async function handleLogout() {
    if (session) {
      try {
        await logout(session.accessToken);
        setSession(null);
        setSessionError("");
      } catch (error: unknown) {
        setSessionError(
          error instanceof Error
            ? error.message
            : "Could not sign out. Please try again.",
        );
        setSession(null);
      }
    }
  }

  if (!session) {
    return (
      <AuthPage
        notice={sessionError}
        onAuthenticated={(nextSession) => {
          setSession(nextSession);
          setSessionError("");
        }}
      />
    );
  }
  if (session.user.role === "student") {
    return <StudentFeedbackPage session={session} onLogout={handleLogout} />;
  }
  if (session.user.role === "administrator") {
    return <AdministratorPage session={session} onLogout={handleLogout} />;
  }
  return <ModeratorApp session={session} onLogout={handleLogout} />;
}

export default App;
