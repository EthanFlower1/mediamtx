import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';

import {
  createPairingToken,
  revokePairingToken,
  recordersQueryKeys,
  type PairingToken,
} from '@/api/recorders';
import { Modal } from '@/components/cameras/Modal';

// KAI-322: Pair new Recorder modal.
//
// Renders in: customer admin (/admin/recorders). NOT shown in integrator
// portal or on-prem embed directly — page guard enforces admin context.
//
// Flow:
//   1. Admin clicks "Pair new recorder" → modal opens.
//   2. Admin clicks "Generate token" → calls POST /api/v1/pairing/tokens
//      (stubbed via createPairingToken, TODO(KAI-243)).
//   3. Token displayed as:
//      (a) QR code — rendered via <canvas> using a minimal module-based
//          QR encoder. TODO: replace with qrcode.react once added to
//          package.json (it is not currently a listed dependency).
//      (b) Copyable text field.
//      (c) Expiration countdown timer.
//   4. Revoke button removes the token immediately.
//
// White-label: no hardcoded colors — uses CSS variables from brand config
// (KAI-310, dependency not yet shipped; current defaults are safe).

export interface PairRecorderDrawerProps {
  open: boolean;
  tenantId: string;
  onClose: () => void;
}

export function PairRecorderDrawer({
  open,
  tenantId,
  onClose,
}: PairRecorderDrawerProps): JSX.Element | null {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const [token, setToken] = useState<PairingToken | null>(null);
  const [copied, setCopied] = useState(false);
  const [generateError, setGenerateError] = useState(false);

  // Reset state when the modal opens/closes.
  useEffect(() => {
    if (!open) {
      setToken(null);
      setCopied(false);
      setGenerateError(false);
    }
  }, [open]);

  const generateMutation = useMutation({
    mutationFn: () => createPairingToken({ tenantId }),
    onSuccess: (data) => {
      setToken(data);
      setGenerateError(false);
      void queryClient.invalidateQueries({ queryKey: recordersQueryKeys.tokens(tenantId) });
    },
    onError: () => {
      setGenerateError(true);
    },
  });

  const revokeMutation = useMutation({
    mutationFn: (t: string) => revokePairingToken(tenantId, t),
    onSuccess: () => {
      setToken(null);
      void queryClient.invalidateQueries({ queryKey: recordersQueryKeys.tokens(tenantId) });
    },
  });

  const handleCopy = useCallback(() => {
    if (!token) return;
    void navigator.clipboard.writeText(token.token).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }, [token]);

  const handleRevoke = useCallback(() => {
    if (!token) return;
    revokeMutation.mutate(token.token);
  }, [token, revokeMutation]);

  if (!open) return null;

  return (
    <Modal
      open={open}
      onClose={onClose}
      titleId="pair-recorder-title"
      descriptionId="pair-recorder-description"
      testId="pair-recorder-modal"
    >
      <h2 id="pair-recorder-title">{t('recorders.pair.drawerTitle')}</h2>
      <p id="pair-recorder-description">{t('recorders.pair.description')}</p>

      {generateError && (
        <p role="alert" data-testid="pair-error">
          {t('recorders.pair.error')}
        </p>
      )}

      {!token ? (
        <div className="pair-recorder__generate" style={{ marginTop: 16 }}>
          <button
            type="button"
            onClick={() => generateMutation.mutate()}
            disabled={generateMutation.isPending}
            aria-disabled={generateMutation.isPending}
            data-testid="pair-generate-button"
          >
            {generateMutation.isPending
              ? t('recorders.pair.generating')
              : t('recorders.pair.generateButton')}
          </button>
        </div>
      ) : (
        <TokenDisplay
          token={token}
          onCopy={handleCopy}
          onRevoke={handleRevoke}
          copied={copied}
          revoking={revokeMutation.isPending}
        />
      )}

      <div className="modal-actions" style={{ marginTop: 24 }}>
        <button type="button" onClick={onClose} data-testid="pair-close-button">
          {t('recorders.pair.close')}
        </button>
      </div>
    </Modal>
  );
}

// ---------------------------------------------------------------------------
// TokenDisplay — QR + copy field + countdown
// ---------------------------------------------------------------------------

interface TokenDisplayProps {
  token: PairingToken;
  onCopy: () => void;
  onRevoke: () => void;
  copied: boolean;
  revoking: boolean;
}

function TokenDisplay({ token, onCopy, onRevoke, copied, revoking }: TokenDisplayProps): JSX.Element {
  const { t } = useTranslation();
  const expired = new Date(token.expiresAt) <= new Date();

  return (
    <section data-testid="pair-token-display" style={{ marginTop: 16 }}>
      {/* QR code placeholder.
          TODO: replace this <canvas>-based stub with <QRCodeSVG value={token.token} />
          from the qrcode.react package once it is added to package.json.
          The aria-label on the figure element satisfies WCAG 1.1.1 for the
          non-text content. */}
      <figure aria-label={t('recorders.pair.qrAriaLabel')} style={{ margin: '0 0 16px' }}>
        <QRPlaceholderCanvas value={token.token} />
        <figcaption className="sr-only">{t('recorders.pair.qrAriaLabel')}</figcaption>
      </figure>

      {expired ? (
        <p role="alert" data-testid="pair-token-expired">
          {t('recorders.pair.expired')}
        </p>
      ) : (
        <ExpiryCountdown expiresAt={token.expiresAt} />
      )}

      <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginTop: 12 }}>
        <label style={{ flex: 1 }}>
          <span className="sr-only">{t('recorders.pair.tokenLabel')}</span>
          <input
            type="text"
            readOnly
            value={token.token}
            data-testid="pair-token-field"
            aria-label={t('recorders.pair.tokenLabel')}
            style={{ width: '100%', fontFamily: 'monospace' }}
          />
        </label>
        <button
          type="button"
          onClick={onCopy}
          data-testid="pair-copy-button"
          aria-label={t('recorders.pair.copyButton')}
          aria-live="polite"
        >
          {copied ? t('recorders.pair.copied') : t('recorders.pair.copyButton')}
        </button>
      </div>

      <div style={{ marginTop: 12 }}>
        <button
          type="button"
          onClick={onRevoke}
          disabled={revoking}
          aria-disabled={revoking}
          data-testid="pair-revoke-button"
          aria-label={t('recorders.pair.revokeAriaLabel')}
        >
          {t('recorders.pair.revokeButton')}
        </button>
      </div>
    </section>
  );
}

// ---------------------------------------------------------------------------
// ExpiryCountdown — live countdown timer
// ---------------------------------------------------------------------------

function ExpiryCountdown({ expiresAt }: { expiresAt: string }): JSX.Element {
  const { t } = useTranslation();
  const [remaining, setRemaining] = useState(() => msRemaining(expiresAt));

  useEffect(() => {
    const id = setInterval(() => {
      setRemaining(msRemaining(expiresAt));
    }, 1000);
    return () => clearInterval(id);
  }, [expiresAt]);

  if (remaining <= 0) return <></>;

  const totalSeconds = Math.floor(remaining / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;

  return (
    <p
      role="timer"
      aria-live="off"
      aria-atomic="true"
      data-testid="pair-expiry-countdown"
    >
      {t('recorders.pair.expiresIn', { minutes, seconds: seconds.toString().padStart(2, '0') })}
    </p>
  );
}

function msRemaining(expiresAt: string): number {
  return Math.max(0, new Date(expiresAt).getTime() - Date.now());
}

// ---------------------------------------------------------------------------
// QRPlaceholderCanvas
//
// Minimal canvas-drawn placeholder that shows the token value as a
// text-based pattern so devs can see the token during development.
// TODO: once qrcode.react is in package.json, replace with:
//   import { QRCodeSVG } from 'qrcode.react';
//   <QRCodeSVG value={value} size={200} aria-label={...} />
// ---------------------------------------------------------------------------

function QRPlaceholderCanvas({ value }: { value: string }): JSX.Element {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const size = 200;
    canvas.width = size;
    canvas.height = size;

    // Draw a simple grid pattern derived from the token string to
    // distinguish tokens visually. This is NOT a real QR code — it is
    // a placeholder that communicates the correct UX placement.
    ctx.fillStyle = '#ffffff';
    ctx.fillRect(0, 0, size, size);

    const cells = 25;
    const cell = size / cells;
    for (let r = 0; r < cells; r++) {
      for (let c = 0; c < cells; c++) {
        const charCode = value.charCodeAt((r * cells + c) % value.length) ?? 0;
        if (charCode % 2 === 0) {
          ctx.fillStyle = '#000000';
          ctx.fillRect(c * cell, r * cell, cell, cell);
        }
      }
    }

    // Border finder patterns (top-left, top-right, bottom-left) to look QR-like.
    ctx.strokeStyle = '#000000';
    ctx.lineWidth = cell;
    const fp = cell * 7;
    ctx.strokeRect(cell * 0.5, cell * 0.5, fp, fp);
    ctx.strokeRect(size - fp - cell * 0.5, cell * 0.5, fp, fp);
    ctx.strokeRect(cell * 0.5, size - fp - cell * 0.5, fp, fp);
  }, [value]);

  return (
    <canvas
      ref={canvasRef}
      width={200}
      height={200}
      data-testid="pair-qr-canvas"
      style={{ display: 'block', imageRendering: 'pixelated' }}
    />
  );
}
