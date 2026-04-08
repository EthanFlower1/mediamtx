// KAI-325: PasswordField — shared masked input component.
//
// Used in all 6 SSO wizards and the user invite form wherever secrets
// are entered. Rules:
//   - Always renders as type="password" by default.
//   - "Show / hide" toggle changes to type="text" temporarily.
//   - Never logs the value.
//   - Forwards ref so react-hook-form register() works.
//   - Renders in: customer admin (wizard dialogs), NOT integrator portal.
//
// WCAG 2.1 AA: toggle button has aria-label and aria-pressed.

import { forwardRef, useState } from 'react';
import type { InputHTMLAttributes } from 'react';

export interface PasswordFieldProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> {
  label: string;
  id: string;
  error?: string;
  masked?: boolean; // true = show the masked sentinel, disable reveal toggle
}

export const PasswordField = forwardRef<HTMLInputElement, PasswordFieldProps>(
  function PasswordField({ label, id, error, masked = false, className, ...rest }, ref) {
    const [revealed, setRevealed] = useState(false);

    return (
      <div className={`password-field${className ? ` ${className}` : ''}`}>
        <label htmlFor={id} className="password-field__label">
          {label}
        </label>
        <div className="password-field__wrapper">
          <input
            {...rest}
            id={id}
            ref={ref}
            type={revealed && !masked ? 'text' : 'password'}
            aria-describedby={error ? `${id}-error` : undefined}
            aria-invalid={error ? 'true' : undefined}
            autoComplete="current-password"
            readOnly={masked}
            className="password-field__input"
          />
          {!masked && (
            <button
              type="button"
              aria-label={revealed ? 'Hide password' : 'Show password'}
              aria-pressed={revealed}
              onClick={() => setRevealed((v) => !v)}
              className="password-field__toggle"
            >
              {revealed ? 'Hide' : 'Show'}
            </button>
          )}
        </div>
        {error && (
          <p id={`${id}-error`} role="alert" className="password-field__error">
            {error}
          </p>
        )}
      </div>
    );
  },
);
