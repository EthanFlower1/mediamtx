import { useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery } from '@tanstack/react-query';

import {
  isValidRtspUrl,
  listRecorders,
  probeProfile,
  scanOnvif,
  type CameraSpec,
  type CameraStreamProfile,
  type OnvifCandidate,
  type Recorder,
  type RetentionTier,
} from '@/api/cameras';
import { Modal } from './Modal';

// KAI-321: Add Camera wizard.
//
// Step 1 — discovery method (ONVIF / manual / QR stub)
// Step 2a — ONVIF scan + candidate picker
// Step 2b — manual entry form (URL + credentials + model hint)
// Step 3 — profile probe (shows main + sub)
// Step 4 — assignment (recorder + retention + schedule)
// Step 5 — review + commit
//
// Credentials are kept in component state only, never logged or
// persisted; the password input uses type="password".

export type DiscoveryMethod = 'onvif' | 'manual' | 'qr';

export interface AddCameraWizardProps {
  open: boolean;
  onClose: () => void;
  tenantId: string;
  onCommit: (spec: CameraSpec) => void;
}

type WizardStep = 1 | 2 | 3 | 4 | 5;

const RETENTION_TIERS: RetentionTier[] = ['short', 'standard', 'long', 'forensic'];

export function AddCameraWizard({
  open,
  onClose,
  tenantId,
  onCommit,
}: AddCameraWizardProps): JSX.Element | null {
  const { t } = useTranslation();
  const [step, setStep] = useState<WizardStep>(1);
  const [method, setMethod] = useState<DiscoveryMethod>('manual');
  const [name, setName] = useState('');
  const [rtspUrl, setRtspUrl] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [modelHint, setModelHint] = useState('');
  const [urlError, setUrlError] = useState<string | null>(null);
  const [selectedCandidate, setSelectedCandidate] = useState<OnvifCandidate | null>(null);
  const [profiles, setProfiles] = useState<CameraStreamProfile[]>([]);
  const [recorderId, setRecorderId] = useState<string>('');
  const [retentionTier, setRetentionTier] = useState<RetentionTier>('standard');
  const [schedule, setSchedule] = useState<'24x7' | 'business'>('24x7');
  const [profileName, setProfileName] = useState('main');

  const recordersQuery = useQuery({
    queryKey: ['cameras', tenantId, 'recorders'],
    queryFn: () => listRecorders(tenantId),
    enabled: open,
  });

  const scanMutation = useMutation({
    mutationFn: () => scanOnvif(),
  });

  const probeMutation = useMutation({
    mutationFn: (args: { rtspUrl: string; username: string; password: string }) =>
      probeProfile(args),
    onSuccess: (res) => setProfiles(res.profiles),
  });

  const reset = useCallback(() => {
    setStep(1);
    setMethod('manual');
    setName('');
    setRtspUrl('');
    setUsername('');
    setPassword('');
    setModelHint('');
    setUrlError(null);
    setSelectedCandidate(null);
    setProfiles([]);
    setRecorderId('');
    setRetentionTier('standard');
    setSchedule('24x7');
    setProfileName('main');
  }, []);

  const handleClose = useCallback(() => {
    reset();
    onClose();
  }, [reset, onClose]);

  const handleCommit = useCallback(() => {
    const spec: CameraSpec = {
      name: name || t('cameras.wizard.defaultName'),
      rtspUrl,
      username,
      password,
      modelHint: modelHint || undefined,
      recorderId: recorderId || (recordersQuery.data?.[0]?.id ?? ''),
      retentionTier,
      profileName,
    };
    onCommit(spec);
    handleClose();
  }, [
    name,
    rtspUrl,
    username,
    password,
    modelHint,
    recorderId,
    recordersQuery.data,
    retentionTier,
    profileName,
    onCommit,
    handleClose,
    t,
  ]);

  if (!open) return null;

  return (
    <Modal
      open={open}
      onClose={handleClose}
      titleId="add-camera-wizard-title"
      testId="add-camera-wizard"
    >
      <header>
        <h2 id="add-camera-wizard-title">{t('cameras.wizard.title')}</h2>
        <p data-testid="wizard-step">
          {t('cameras.wizard.stepIndicator', { step, total: 5 })}
        </p>
      </header>

      {step === 1 && (
        <section aria-label={t('cameras.wizard.step1.sectionLabel')}>
          <h3>{t('cameras.wizard.step1.heading')}</h3>
          <fieldset>
            <legend>{t('cameras.wizard.step1.legend')}</legend>
            <label>
              <input
                type="radio"
                name="discovery-method"
                checked={method === 'onvif'}
                onChange={() => setMethod('onvif')}
                data-testid="wizard-method-onvif"
              />
              {t('cameras.wizard.step1.onvif')}
            </label>
            <label>
              <input
                type="radio"
                name="discovery-method"
                checked={method === 'manual'}
                onChange={() => setMethod('manual')}
                data-testid="wizard-method-manual"
              />
              {t('cameras.wizard.step1.manual')}
            </label>
            <label>
              <input
                type="radio"
                name="discovery-method"
                checked={method === 'qr'}
                onChange={() => setMethod('qr')}
                data-testid="wizard-method-qr"
                disabled
                aria-describedby="qr-stub-desc"
              />
              {t('cameras.wizard.step1.qr')}
            </label>
            <p id="qr-stub-desc">{t('cameras.wizard.step1.qrStub')}</p>
          </fieldset>
          <div className="wizard-actions">
            <button type="button" onClick={handleClose}>
              {t('cameras.wizard.cancel')}
            </button>
            <button
              type="button"
              data-testid="wizard-next"
              onClick={() => {
                setStep(2);
                if (method === 'onvif') scanMutation.mutate();
              }}
            >
              {t('cameras.wizard.next')}
            </button>
          </div>
        </section>
      )}

      {step === 2 && method === 'onvif' && (
        <section aria-label={t('cameras.wizard.step2.onvifSectionLabel')}>
          <h3>{t('cameras.wizard.step2.onvifHeading')}</h3>
          {scanMutation.isPending && (
            <p role="status" aria-live="polite" data-testid="onvif-scan-progress">
              {t('cameras.wizard.step2.scanning')}
            </p>
          )}
          {scanMutation.isSuccess && (
            <ul role="list" data-testid="onvif-candidates">
              {scanMutation.data.map((cand) => (
                <li key={cand.id}>
                  <label>
                    <input
                      type="radio"
                      name="onvif-candidate"
                      checked={selectedCandidate?.id === cand.id}
                      onChange={() => {
                        setSelectedCandidate(cand);
                        setRtspUrl(`rtsp://${cand.ipAddress}:554/Streaming/Channels/0`);
                        setModelHint(cand.model);
                      }}
                      data-testid={`onvif-candidate-${cand.id}`}
                    />
                    {cand.vendor} {cand.model} — {cand.ipAddress}
                  </label>
                </li>
              ))}
            </ul>
          )}
          <div className="wizard-actions">
            <button type="button" onClick={() => setStep(1)}>
              {t('cameras.wizard.back')}
            </button>
            <button
              type="button"
              data-testid="wizard-next"
              disabled={!selectedCandidate}
              onClick={() => setStep(3)}
            >
              {t('cameras.wizard.next')}
            </button>
          </div>
        </section>
      )}

      {step === 2 && method === 'manual' && (
        <section aria-label={t('cameras.wizard.step2.manualSectionLabel')}>
          <h3>{t('cameras.wizard.step2.manualHeading')}</h3>
          <label>
            {t('cameras.wizard.fields.name')}
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              data-testid="wizard-field-name"
            />
          </label>
          <label>
            {t('cameras.wizard.fields.rtspUrl')}
            <input
              type="url"
              value={rtspUrl}
              onChange={(e) => {
                setRtspUrl(e.target.value);
                setUrlError(null);
              }}
              aria-invalid={urlError ? 'true' : undefined}
              aria-describedby={urlError ? 'wizard-url-error' : undefined}
              data-testid="wizard-field-rtsp"
            />
          </label>
          {urlError && (
            <p id="wizard-url-error" role="alert" data-testid="wizard-url-error">
              {urlError}
            </p>
          )}
          <label>
            {t('cameras.wizard.fields.username')}
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              data-testid="wizard-field-username"
            />
          </label>
          <label>
            {t('cameras.wizard.fields.password')}
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              data-testid="wizard-field-password"
              autoComplete="new-password"
            />
          </label>
          <label>
            {t('cameras.wizard.fields.modelHint')}
            <input
              type="text"
              value={modelHint}
              onChange={(e) => setModelHint(e.target.value)}
              data-testid="wizard-field-model-hint"
            />
          </label>
          <div className="wizard-actions">
            <button type="button" onClick={() => setStep(1)}>
              {t('cameras.wizard.back')}
            </button>
            <button
              type="button"
              data-testid="wizard-next"
              onClick={() => {
                if (!isValidRtspUrl(rtspUrl)) {
                  setUrlError(t('cameras.wizard.errors.invalidRtsp'));
                  return;
                }
                probeMutation.mutate({ rtspUrl, username, password });
                setStep(3);
              }}
            >
              {t('cameras.wizard.next')}
            </button>
          </div>
        </section>
      )}

      {step === 3 && (
        <section aria-label={t('cameras.wizard.step3.sectionLabel')}>
          <h3>{t('cameras.wizard.step3.heading')}</h3>
          {probeMutation.isPending && (
            <p role="status">{t('cameras.wizard.step3.probing')}</p>
          )}
          {(profiles.length > 0 || probeMutation.isSuccess) && (
            <ul role="list" data-testid="wizard-profiles">
              {profiles.map((p) => (
                <li key={p.name}>
                  <label>
                    <input
                      type="radio"
                      name="wizard-profile"
                      checked={profileName === p.name}
                      onChange={() => setProfileName(p.name)}
                      data-testid={`wizard-profile-${p.name}`}
                    />
                    {p.name} — {p.resolution} @ {p.fps}fps ({p.codec})
                  </label>
                </li>
              ))}
            </ul>
          )}
          <div className="wizard-actions">
            <button type="button" onClick={() => setStep(2)}>
              {t('cameras.wizard.back')}
            </button>
            <button
              type="button"
              data-testid="wizard-next"
              onClick={() => setStep(4)}
            >
              {t('cameras.wizard.next')}
            </button>
          </div>
        </section>
      )}

      {step === 4 && (
        <section aria-label={t('cameras.wizard.step4.sectionLabel')}>
          <h3>{t('cameras.wizard.step4.heading')}</h3>
          <label>
            {t('cameras.wizard.fields.recorder')}
            <select
              value={recorderId}
              onChange={(e) => setRecorderId(e.target.value)}
              data-testid="wizard-field-recorder"
            >
              <option value="">{t('cameras.wizard.step4.pickRecorder')}</option>
              {(recordersQuery.data ?? []).map((r: Recorder) => (
                <option key={r.id} value={r.id}>
                  {r.name} — {r.cameraCount}/{r.capacity}
                </option>
              ))}
            </select>
          </label>
          <label>
            {t('cameras.wizard.fields.retention')}
            <select
              value={retentionTier}
              onChange={(e) => setRetentionTier(e.target.value as RetentionTier)}
              data-testid="wizard-field-retention"
            >
              {RETENTION_TIERS.map((tier) => (
                <option key={tier} value={tier}>
                  {t(`cameras.retention.${tier}`)}
                </option>
              ))}
            </select>
          </label>
          <label>
            {t('cameras.wizard.fields.schedule')}
            <select
              value={schedule}
              onChange={(e) => setSchedule(e.target.value as '24x7' | 'business')}
              data-testid="wizard-field-schedule"
            >
              <option value="24x7">{t('cameras.wizard.schedule.24x7')}</option>
              <option value="business">{t('cameras.wizard.schedule.business')}</option>
            </select>
          </label>
          <div className="wizard-actions">
            <button type="button" onClick={() => setStep(3)}>
              {t('cameras.wizard.back')}
            </button>
            <button
              type="button"
              data-testid="wizard-next"
              onClick={() => setStep(5)}
            >
              {t('cameras.wizard.next')}
            </button>
          </div>
        </section>
      )}

      {step === 5 && (
        <section aria-label={t('cameras.wizard.step5.sectionLabel')}>
          <h3>{t('cameras.wizard.step5.heading')}</h3>
          <dl data-testid="wizard-review">
            <dt>{t('cameras.wizard.fields.name')}</dt>
            <dd>{name || t('cameras.wizard.defaultName')}</dd>
            <dt>{t('cameras.wizard.fields.rtspUrl')}</dt>
            <dd>{rtspUrl}</dd>
            <dt>{t('cameras.wizard.fields.recorder')}</dt>
            <dd>
              {recordersQuery.data?.find((r) => r.id === recorderId)?.name ??
                t('cameras.wizard.step4.pickRecorder')}
            </dd>
            <dt>{t('cameras.wizard.fields.retention')}</dt>
            <dd>{t(`cameras.retention.${retentionTier}`)}</dd>
            <dt>{t('cameras.wizard.fields.schedule')}</dt>
            <dd>{t(`cameras.wizard.schedule.${schedule}`)}</dd>
          </dl>
          <div className="wizard-actions">
            <button type="button" onClick={() => setStep(4)}>
              {t('cameras.wizard.back')}
            </button>
            <button
              type="button"
              data-testid="wizard-commit"
              onClick={handleCommit}
            >
              {t('cameras.wizard.commit')}
            </button>
          </div>
        </section>
      )}
    </Modal>
  );
}
