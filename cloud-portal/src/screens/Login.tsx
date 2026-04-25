import { useState, type FormEvent } from 'react';
import { Navigate, useLocation, useNavigate } from 'react-router-dom';
import logo from '../assets/logo.png';
import { useSession } from '../auth/SessionContext';
import { Btn } from '../components/primitives';

interface LocationState { from?: { pathname: string } }

export function Login() {
  const { status, signIn } = useSession();
  const location = useLocation();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const from = (location.state as LocationState | null)?.from?.pathname ?? '/overview';

  if (status === 'authenticated') return <Navigate to={from} replace />;

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await signIn(email.trim(), password);
      navigate(from, { replace: true });
    } catch {
      setError('Invalid email or password.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div style={{
      flex: 1, height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'var(--bg-primary)',
    }}>
      <form onSubmit={onSubmit} style={{
        width: 380, padding: 32, background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6,
        display: 'flex', flexDirection: 'column', gap: 18,
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 6 }}>
          <img src={logo} alt="" style={{ width: 28, height: 28 }} />
          <div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 14, letterSpacing: 3, fontWeight: 600, color: 'var(--text-primary)' }}>RAIKADA</div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: 1, color: 'var(--text-muted)' }}>CLOUD PORTAL</div>
          </div>
        </div>

        <Field
          label="Email"
          type="email"
          autoComplete="username"
          value={email}
          onChange={setEmail}
          required
        />
        <Field
          label="Password"
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={setPassword}
          required
        />

        {error && (
          <div style={{
            padding: '8px 10px', background: 'rgba(239,68,68,0.07)', border: '1px solid rgba(239,68,68,0.27)', borderRadius: 4,
            fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--danger)', letterSpacing: 0.5,
          }}>{error}</div>
        )}

        <Btn kind="primary" style={{ width: '100%', justifyContent: 'center' }}>
          {submitting ? 'Signing in…' : 'Sign in'}
        </Btn>

        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 0.5, color: 'var(--text-muted)', textAlign: 'center', marginTop: 4 }}>
          cloud.raikada.com · multi-site control plane
        </div>
      </form>
    </div>
  );
}

interface FieldProps {
  label: string;
  type: string;
  value: string;
  onChange: (v: string) => void;
  autoComplete?: string;
  required?: boolean;
}

function Field({ label, type, value, onChange, autoComplete, required }: FieldProps) {
  return (
    <label style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <span style={{
        fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1, color: 'var(--text-secondary)', textTransform: 'uppercase',
      }}>{label}</span>
      <input
        type={type}
        value={value}
        autoComplete={autoComplete}
        required={required}
        onChange={(e) => onChange(e.target.value)}
        style={{
          height: 36, padding: '0 10px', background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 4,
          color: 'var(--text-primary)', fontFamily: type === 'password' ? 'var(--font-mono)' : 'var(--font-sans)',
          fontSize: 13, outline: 'none',
        }}
      />
    </label>
  );
}
