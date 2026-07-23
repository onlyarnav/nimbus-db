'use client';

import { useEffect, useState } from 'react';

interface NodeInfo {
  id: string;
  cluster_id: string;
  hostname: string;
  status: string;
  cpu_pct: number;
  memory_pct: number;
  disk_pct: number;
  last_heartbeat: string | null;
  registered_at: string;
}

interface RegionInfo {
  region: string;
  status: 'healthy' | 'degraded' | 'down';
  total_nodes: number;
  healthy_nodes: number;
  unhealthy_nodes: number;
  dead_nodes: number;
}

const STATIC_REGIONS = ['india', 'us-east', 'us-west', 'europe', 'japan'];

const LATENCY_MATRIX: Record<string, Record<string, number>> = {
  'india': { 'india': 0, 'us-east': 180, 'us-west': 220, 'europe': 110, 'japan': 130 },
  'us-east': { 'india': 180, 'us-east': 0, 'us-west': 60, 'europe': 90, 'japan': 160 },
  'us-west': { 'india': 220, 'us-east': 60, 'us-west': 0, 'europe': 140, 'japan': 110 },
  'europe': { 'india': 110, 'us-east': 90, 'us-west': 140, 'europe': 0, 'japan': 210 },
  'japan': { 'india': 130, 'us-east': 160, 'us-west': 110, 'europe': 210, 'japan': 0 },
};

export default function Home() {
  const [nodes, setNodes] = useState<NodeInfo[]>([]);
  const [regions, setRegions] = useState<RegionInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchNodesAndRegions = async () => {
    try {
      const res = await fetch('http://localhost:8080/v1/nodes');
      if (!res.ok) {
        throw new Error(`Failed to fetch nodes: ${res.statusText}`);
      }
      const data: NodeInfo[] = await res.json();
      setNodes(data);

      // Compute Region Health Rollup
      const rList: RegionInfo[] = STATIC_REGIONS.map((reg) => {
        const regNodes = data.filter((n) =>
          n.cluster_id.toLowerCase().includes(reg) || n.hostname.toLowerCase().includes(reg)
        );
        const total = regNodes.length;
        const healthy = regNodes.filter((n) => n.status.toLowerCase() === 'healthy').length;
        const unhealthy = regNodes.filter((n) => n.status.toLowerCase() === 'unhealthy').length;
        const dead = regNodes.filter((n) => n.status.toLowerCase() === 'dead').length;

        let status: 'healthy' | 'degraded' | 'down' = 'down';
        if (total > 0 && healthy > 0) {
          status = (unhealthy > 0 || dead > 0) ? 'degraded' : 'healthy';
        }

        return {
          region: reg,
          status,
          total_nodes: total,
          healthy_nodes: healthy,
          unhealthy_nodes: unhealthy,
          dead_nodes: dead,
        };
      });

      setRegions(rList);
      setError(null);
    } catch (err: any) {
      setError(err.message || 'Connection to Metadata Service failed');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchNodesAndRegions();
    const interval = setInterval(fetchNodesAndRegions, 3000);
    return () => clearInterval(interval);
  }, []);

  const getStatusColor = (status: string) => {
    switch (status.toLowerCase()) {
      case 'healthy':
        return '#10B981'; // Green
      case 'degraded':
      case 'unhealthy':
        return '#F59E0B'; // Orange
      case 'down':
      case 'dead':
        return '#EF4444'; // Red
      case 'overloaded':
        return '#EC4899'; // Pink/Purple
      default:
        return '#6B7280'; // Gray
    }
  };

  const getRelativeTime = (timestamp: string | null) => {
    if (!timestamp) return 'Never';
    const now = new Date();
    const diff = now.getTime() - new Date(timestamp).getTime();
    const seconds = Math.floor(diff / 1000);
    if (seconds < 0) return '0s ago';
    if (seconds < 60) return `${seconds}s ago`;
    const minutes = Math.floor(seconds / 60);
    return `${minutes}m ago`;
  };

  return (
    <div style={{ fontFamily: 'sans-serif', backgroundColor: '#0F172A', color: '#F8FAFC', minHeight: '100vh', padding: '2rem' }}>
      <header style={{ borderBottom: '1px solid #334155', paddingBottom: '1rem', marginBottom: '2rem' }}>
        <h1 style={{ fontSize: '2rem', margin: 0, fontWeight: 700 }}>NimbusDB Multi-Region Control Plane</h1>
        <p style={{ color: '#94A3B8', margin: '0.5rem 0 0 0' }}>Live Multi-Region & Worker Node Topology Monitoring</p>
      </header>

      {error && (
        <div style={{ backgroundColor: '#7F1D1D', color: '#FCA5A5', padding: '1rem', borderRadius: '0.375rem', marginBottom: '1.5rem' }}>
          <strong>Error:</strong> {error}. Ensure Metadata Service is running on http://localhost:8080
        </div>
      )}

      {/* Region-Level Health View */}
      <section style={{ marginBottom: '3rem' }}>
        <h2 style={{ fontSize: '1.5rem', marginBottom: '1rem', color: '#38BDF8' }}>Region-Level Health Status</h2>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))', gap: '1rem', marginBottom: '1.5rem' }}>
          {regions.map((reg) => (
            <div key={reg.region} style={{ backgroundColor: '#1E293B', borderRadius: '0.5rem', border: '1px solid #334155', padding: '1rem' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.5rem' }}>
                <strong style={{ fontSize: '1.1rem', textTransform: 'uppercase' }}>{reg.region}</strong>
                <span style={{
                  backgroundColor: getStatusColor(reg.status),
                  color: '#FFFFFF',
                  padding: '0.2rem 0.5rem',
                  borderRadius: '9999px',
                  fontSize: '0.7rem',
                  fontWeight: 700,
                  textTransform: 'uppercase'
                }}>
                  {reg.status}
                </span>
              </div>
              <div style={{ fontSize: '0.85rem', color: '#94A3B8' }}>
                <div>Total Nodes: {reg.total_nodes}</div>
                <div>Healthy: {reg.healthy_nodes} | Dead: {reg.dead_nodes}</div>
              </div>
            </div>
          ))}
        </div>

        {/* Latency Matrix */}
        <div style={{ backgroundColor: '#1E293B', borderRadius: '0.5rem', border: '1px solid #334155', padding: '1rem' }}>
          <h3 style={{ margin: '0 0 0.5rem 0', fontSize: '1rem', color: '#94A3B8' }}>Synthetic Inter-Region Latency Matrix (ms)</h3>
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.85rem' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid #334155' }}>
                  <th style={{ padding: '0.5rem', textAlign: 'left' }}>From / To</th>
                  {STATIC_REGIONS.map((r) => (
                    <th key={r} style={{ padding: '0.5rem', textAlign: 'center' }}>{r}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {STATIC_REGIONS.map((from) => (
                  <tr key={from} style={{ borderBottom: '1px solid #334155' }}>
                    <td style={{ padding: '0.5rem', fontWeight: 600 }}>{from}</td>
                    {STATIC_REGIONS.map((to) => (
                      <td key={to} style={{ padding: '0.5rem', textAlign: 'center', color: from === to ? '#10B981' : '#CBD5E1' }}>
                        {LATENCY_MATRIX[from][to]}ms
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </section>

      {/* Node-Level View */}
      <section>
        <h2 style={{ fontSize: '1.5rem', marginBottom: '1rem', color: '#38BDF8' }}>Worker Nodes Telemetry</h2>
        {loading ? (
          <div style={{ fontSize: '1.2rem', color: '#94A3B8' }}>Loading cluster status...</div>
        ) : nodes.length === 0 ? (
          <div style={{ fontSize: '1.2rem', color: '#94A3B8' }}>No worker nodes registered yet.</div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))', gap: '1.5rem' }}>
            {nodes.map((node) => (
              <div key={node.id} style={{ backgroundColor: '#1E293B', borderRadius: '0.5rem', border: '1px solid #334155', padding: '1.5rem', boxShadow: '0 4px 6px -1px rgba(0, 0, 0, 0.1)' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
                  <h3 style={{ margin: 0, fontSize: '1.25rem', fontWeight: 600 }}>{node.hostname}</h3>
                  <span style={{
                    backgroundColor: getStatusColor(node.status),
                    color: '#FFFFFF',
                    padding: '0.25rem 0.75rem',
                    borderRadius: '9999px',
                    fontSize: '0.75rem',
                    fontWeight: 700,
                    textTransform: 'uppercase'
                  }}>
                    {node.status}
                  </span>
                </div>

                <div style={{ color: '#94A3B8', fontSize: '0.875rem', marginBottom: '1rem' }}>
                  <div style={{ textOverflow: 'ellipsis', overflow: 'hidden', whiteSpace: 'nowrap' }}><strong>Cluster / Region ID:</strong> {node.cluster_id}</div>
                  <div style={{ textOverflow: 'ellipsis', overflow: 'hidden', whiteSpace: 'nowrap' }}><strong>Node ID:</strong> {node.id}</div>
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem', marginBottom: '1rem' }}>
                  <div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.875rem', marginBottom: '0.25rem' }}>
                      <span>CPU Usage</span>
                      <span>{node.cpu_pct.toFixed(1)}%</span>
                    </div>
                    <div style={{ height: '6px', backgroundColor: '#334155', borderRadius: '3px', overflow: 'hidden' }}>
                      <div style={{ width: `${node.cpu_pct}%`, height: '100%', backgroundColor: node.cpu_pct > 90 ? '#EC4899' : '#3B82F6' }}></div>
                    </div>
                  </div>

                  <div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.875rem', marginBottom: '0.25rem' }}>
                      <span>Memory Usage</span>
                      <span>{node.memory_pct.toFixed(1)}%</span>
                    </div>
                    <div style={{ height: '6px', backgroundColor: '#334155', borderRadius: '3px', overflow: 'hidden' }}>
                      <div style={{ width: `${node.memory_pct}%`, height: '100%', backgroundColor: node.memory_pct > 90 ? '#EC4899' : '#10B981' }}></div>
                    </div>
                  </div>

                  <div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.875rem', marginBottom: '0.25rem' }}>
                      <span>Disk Space</span>
                      <span>{node.disk_pct.toFixed(1)}%</span>
                    </div>
                    <div style={{ height: '6px', backgroundColor: '#334155', borderRadius: '3px', overflow: 'hidden' }}>
                      <div style={{ width: `${node.disk_pct}%`, height: '100%', backgroundColor: node.disk_pct > 90 ? '#EC4899' : '#F59E0B' }}></div>
                    </div>
                  </div>
                </div>

                <div style={{ borderTop: '1px solid #334155', paddingTop: '0.75rem', fontSize: '0.875rem', color: '#94A3B8', display: 'flex', justifyContent: 'space-between' }}>
                  <span>Last Heartbeat:</span>
                  <span>{getRelativeTime(node.last_heartbeat)}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
