import { NextResponse } from 'next/server';

/**
 * HubSpot lead capture stub.
 *
 * TODO(KAI-342 follow-up): wire to HubSpot Forms API:
 *   POST https://api.hsforms.com/submissions/v3/integration/submit/{portalId}/{formGuid}
 *
 * Until HUBSPOT_PORTAL_ID and HUBSPOT_FORM_GUID are real, this endpoint
 * just logs the payload and returns 200. NEVER call HubSpot's real API
 * during scaffold/dev — it will pollute the CRM.
 */
export async function POST(request: Request): Promise<NextResponse> {
  let payload: unknown;
  try {
    payload = await request.json();
  } catch {
    return NextResponse.json({ ok: false, error: 'invalid_json' }, { status: 400 });
  }

  // eslint-disable-next-line no-console
  console.log('[lead] captured (stub, not sent to HubSpot):', payload);

  const portalId = process.env.HUBSPOT_PORTAL_ID;
  const formGuid = process.env.HUBSPOT_FORM_GUID;
  if (!portalId || !formGuid || portalId === 'REPLACE_ME' || formGuid === 'REPLACE_ME') {
    return NextResponse.json({ ok: true, mode: 'stub' }, { status: 200 });
  }

  // Real implementation lives here once credentials are provisioned.
  return NextResponse.json({ ok: true, mode: 'stub' }, { status: 200 });
}
