import { NextResponse } from 'next/server';

export const dynamic = 'force-dynamic';

export async function GET() {
  const gatewayUrl = process.env.GATEWAY_URL || 'http://nimbusdb-gateway:8080';
  const metadataUrl = process.env.METADATA_SERVICE_URL || 'http://nimbusdb-metadata-service:8080';

  // 1. Try Gateway /v1/nodes
  try {
    const res = await fetch(`${gatewayUrl}/v1/nodes`, {
      headers: { 'Accept': 'application/json' },
      cache: 'no-store',
    });
    if (res.ok) {
      const data = await res.json();
      return NextResponse.json(data);
    }
  } catch {}

  // 2. Try Metadata Service /v1/nodes
  try {
    const res = await fetch(`${metadataUrl}/v1/nodes`, {
      headers: { 'Accept': 'application/json' },
      cache: 'no-store',
    });
    if (res.ok) {
      const data = await res.json();
      return NextResponse.json(data);
    }
  } catch {}

  // 3. Try Localhost fallback (when dashboard is running outside cluster)
  try {
    const res = await fetch('http://localhost:8080/v1/nodes', {
      headers: { 'Accept': 'application/json' },
      cache: 'no-store',
    });
    if (res.ok) {
      const data = await res.json();
      return NextResponse.json(data);
    }
  } catch {}

  return NextResponse.json([]);
}
