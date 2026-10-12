import { useState, type FormEvent } from "react";
import {
  login,
  requestRegistrationOTP,
  verifyOTP,
  type AuthSession,
  type UserRole,
} from "../services/auth";

interface AuthPageProps {
  onAuthenticated: (session: AuthSession) => void;
  notice: string;
}

type AuthMode = "login" | "register";
type LoginRole = Extract<UserRole, "student" | "administrator" | "moderator">;

const emailPattern = /^[^\s@]+@iitk\.ac\.in$/i;

function AuthPage({ onAuthenticated, notice }: AuthPageProps) {
  const [mode, setMode] = useState<AuthMode>("login");
  const [loginRole, setLoginRole] = useState<LoginRole>("student");
  const [step, setStep] = useState<"form" | "verify">("form");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [code, setCode] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState("");

  function resetForm(nextMode: AuthMode) {
    setMode(nextMode);
    setStep("form");
    setPassword("");
    setConfirmPassword("");
    setCode("");
    setError("");
  }

  function selectLoginRole(nextRole: LoginRole) {
    setLoginRole(nextRole);
    resetForm("login");
  }

  function validateRegistration() {
    if (!emailPattern.test(email.trim())) {
      return "Use a valid IITK email address ending in @iitk.ac.in.";
    }
    if (password.length < 6 || password.length > 72) {
      return "Your password must be between 6 and 72 characters.";
    }
    if (password !== confirmPassword) {
      return "The passwords do not match.";
    }
    return "";
  }

  function validateLogin() {
    if (!/^\S+@\S+\.\S+$/.test(email.trim())) {
      return "Enter a valid email address.";
    }
    if (!password) {
      return "Enter your password to continue.";
    }
    return "";
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");

    if (mode === "register" && step === "verify") {
      if (!/^\d{6}$/.test(code)) {
        setError("Enter the six-digit verification code from your email.");
        return;
      }
    } else {
      const validationError = mode === "register"
        ? validateRegistration()
        : validateLogin();
      if (validationError) {
        setError(validationError);
        return;
      }
    }

    setIsSubmitting(true);
    try {
      const normalizedEmail = email.trim().toLowerCase();
      if (mode === "login") {
        onAuthenticated(await login(normalizedEmail, password, loginRole));
      } else if (step === "form") {
        await requestRegistrationOTP(normalizedEmail, password, confirmPassword);
        setEmail(normalizedEmail);
        setStep("verify");
      } else {
        onAuthenticated(await verifyOTP(normalizedEmail, code, password));
      }
    } catch (requestError: unknown) {
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Authentication failed. Please try again.",
      );
    } finally {
      setIsSubmitting(false);
    }
  }

  const isVerificationStep = mode === "register" && step === "verify";

  return (
    <main className="auth-shell">
      <section className="auth-intro">
        <div className="auth-brand">
          <span className="auth-brand-mark">C</span>
          <span>CampusEcho</span>
        </div>
        <div className="auth-intro-copy">
          <p className="auth-kicker">IIT Kanpur · private feedback</p>
          <h1>Speak clearly.<br /><em>Be heard</em> safely.</h1>
          <p className="auth-lede">
            A focused space for students to share thoughtful course feedback,
            with identity kept separate from every submission.
          </p>
        </div>
        <div className="auth-intro-footer">
          <span className="auth-status-dot" />
          <span>Secure campus access</span>
          <span className="auth-footer-line" />
          <span>v1.0</span>
        </div>
      </section>

      <section className="auth-panel">
        <div className="auth-panel-inner">
          <div className="auth-mobile-brand auth-brand">
            <span className="auth-brand-mark">C</span>
            <span>CampusEcho</span>
          </div>

          <div className="auth-heading">
            <p className="auth-kicker">{isVerificationStep ? "One last step" : "Welcome back"}</p>
            <h2>
              {isVerificationStep
                ? "Verify your email"
                : mode === "login" ? `${loginRole === "administrator" ? "Administrator" : loginRole === "moderator" ? "Moderator" : "Student"} sign in` : "Create your account"}
            </h2>
            <p>
              {isVerificationStep
                ? `We sent a six-digit code to ${email}.`
                : mode === "login"
                  ? `Use your ${loginRole === "administrator" ? "administrator" : loginRole === "moderator" ? "moderator" : "student"} credentials to access CampusEcho.`
                  : "Register with your IITK email to get started."}
            </p>
            {!isVerificationStep && mode === "login" && (
              <p className="auth-role-hint">{loginRole === "student" ? "Students register with an IITK email and verify it by OTP." : "This account must be created by an administrator."}</p>
            )}
          </div>

          {!isVerificationStep && mode === "login" && (
            <div className="auth-role-tabs" role="tablist" aria-label="Account role">
              <button className={loginRole === "student" ? "auth-role-tab active" : "auth-role-tab"} onClick={() => selectLoginRole("student")} role="tab" aria-selected={loginRole === "student"} type="button">Student</button>
              <button className={loginRole === "administrator" ? "auth-role-tab active" : "auth-role-tab"} onClick={() => selectLoginRole("administrator")} role="tab" aria-selected={loginRole === "administrator"} type="button">Administrator</button>
              <button className={loginRole === "moderator" ? "auth-role-tab active" : "auth-role-tab"} onClick={() => selectLoginRole("moderator")} role="tab" aria-selected={loginRole === "moderator"} type="button">Moderator</button>
            </div>
          )}

          {!isVerificationStep && (
            <div className="auth-tabs" role="tablist" aria-label="Authentication mode">
              <button className={mode === "login" ? "auth-tab active" : "auth-tab"} onClick={() => resetForm("login")} role="tab" aria-selected={mode === "login"} type="button">Sign in</button>
              {loginRole === "student" && <button className={mode === "register" ? "auth-tab active" : "auth-tab"} onClick={() => resetForm("register")} role="tab" aria-selected={mode === "register"} type="button">Register</button>}
            </div>
          )}

          {notice && <p className="auth-notice" role="status">{notice}</p>}

          <form className="auth-form" onSubmit={handleSubmit}>
            {isVerificationStep ? (
              <label className="auth-field">
                <span>Verification code</span>
                <input autoComplete="one-time-code" autoFocus className="auth-input auth-code-input" inputMode="numeric" maxLength={6} onChange={(event) => setCode(event.target.value.replace(/\D/g, ""))} placeholder="000000" value={code} />
                <small>Check your inbox. The code expires in 10 minutes.</small>
              </label>
            ) : (
              <>
                <label className="auth-field">
                  <span>{mode === "register" ? "IITK email" : "Email address"}</span>
                  <input autoComplete="email" className="auth-input" onChange={(event) => setEmail(event.target.value)} placeholder={mode === "register" ? "you@iitk.ac.in" : "you@example.com"} value={email} type="email" />
                </label>
                <label className="auth-field">
                  <span>Password</span>
                  <input autoComplete={mode === "login" ? "current-password" : "new-password"} className="auth-input" maxLength={72} onChange={(event) => setPassword(event.target.value)} placeholder="6 characters minimum" value={password} type="password" />
                  {mode === "register" && <small>Use 6–72 characters.</small>}
                </label>
                {mode === "register" && (
                  <label className="auth-field">
                    <span>Confirm password</span>
                    <input autoComplete="new-password" className="auth-input" maxLength={72} onChange={(event) => setConfirmPassword(event.target.value)} placeholder="Re-enter your password" value={confirmPassword} type="password" />
                  </label>
                )}
              </>
            )}

            {error && <p className="auth-error" role="alert">{error}</p>}

            <button className="auth-submit" disabled={isSubmitting} type="submit">
              <span>{isSubmitting
                ? isVerificationStep ? "Verifying..." : mode === "register" ? "Sending code..." : "Signing in..."
                : isVerificationStep ? "Verify and create account" : mode === "register" ? "Send verification code" : "Sign in"}</span>
            </button>
          </form>

          {isVerificationStep && <button className="auth-back" onClick={() => setStep("form")} type="button">Edit registration details</button>}
          <p className="auth-privacy">By continuing, you agree to use your verified IITK identity responsibly.</p>
        </div>
      </section>
    </main>
  );
}

export default AuthPage;
