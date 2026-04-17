import { useState, useEffect, useCallback } from 'react';
import {
  Box, Typography, Tabs, Tab, Button, Table, TableBody, TableCell,
  TableHead, TableRow, TextField, Chip, IconButton, Snackbar, Alert,
  Select, MenuItem, FormControl, InputLabel, Divider,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import HubIcon from '@mui/icons-material/Hub';
import LanIcon from '@mui/icons-material/Lan';
import SecurityIcon from '@mui/icons-material/Security';
import DnsIcon from '@mui/icons-material/Dns';
import { useProjectContext } from '../contexts/ProjectContext';

type VpcNetwork  = { name: string; autoCreateSubnetworks: boolean; creationTimestamp: string };
type Firewall    = { name: string; direction: string; priority: number; sourceRanges: string[]; destinationRanges: string[]; allowed: { IPProtocol: string; ports: string[] }[]; denied: { IPProtocol: string; ports: string[] }[]; disabled: boolean; description: string; creationTimestamp: string };
type ManagedZone = { name: string; dnsName: string; visibility: string; nameServers: string[] };
type RRSet       = { name: string; type: string; ttl: number; rrdatas: string[] };

export default function NetworkPage() {
  const { activeProject } = useProjectContext();
  const [tab, setTab] = useState(0);
  const [toast, setToast] = useState({ msg: '', open: false, severity: 'success' as 'success' | 'error' });
  const showToast = (msg: string, severity: 'success' | 'error' = 'success') => setToast({ msg, open: true, severity });

  // ── VPC ──────────────────────────────────────────────────────────────────────
  const [networks, setNetworks]   = useState<VpcNetwork[]>([]);
  const [vpcName, setVpcName]     = useState('');
  const [autoSubnet, setAutoSubnet] = useState(true);

  // ── Firewall ─────────────────────────────────────────────────────────────────
  const [firewalls, setFirewalls]   = useState<Firewall[]>([]);
  const [fwName, setFwName]         = useState('');
  const [fwDirection, setFwDirection] = useState('INGRESS');
  const [fwProtocol, setFwProtocol] = useState('tcp');
  const [fwPorts, setFwPorts]       = useState('80,443');
  const [fwSource, setFwSource]     = useState('0.0.0.0/0');
  const [fwAction, setFwAction]     = useState('allow');

  // ── DNS ──────────────────────────────────────────────────────────────────────
  const [zones, setZones]           = useState<ManagedZone[]>([]);
  const [selectedZone, setSelectedZone] = useState<ManagedZone | null>(null);
  const [rrsets, setRrsets]         = useState<RRSet[]>([]);
  const [newZoneName, setNewZoneName] = useState('');
  const [newDnsName, setNewDnsName]   = useState('');
  const [newRRName, setNewRRName]     = useState('');
  const [newRRType, setNewRRType]     = useState('A');
  const [newRRData, setNewRRData]     = useState('');
  const [newRRTtl, setNewRRTtl]       = useState('300');

  // ── Loaders ──────────────────────────────────────────────────────────────────
  const loadNetworks = useCallback(async () => {
    const res = await fetch(`/api/manage/network/projects/${activeProject}/global/networks`);
    if (res.ok) setNetworks((await res.json()).items || []);
  }, [activeProject]);

  const loadFirewalls = useCallback(async () => {
    const res = await fetch(`/api/manage/network/projects/${activeProject}/global/firewalls`);
    if (res.ok) setFirewalls((await res.json()).items || []);
  }, [activeProject]);

  const loadZones = useCallback(async () => {
    const res = await fetch(`/api/manage/dns/projects/${activeProject}/managedZones`);
    if (res.ok) setZones((await res.json()).managedZones || []);
  }, [activeProject]);

  const loadRRSets = useCallback(async (zoneName: string) => {
    const res = await fetch(`/api/manage/dns/projects/${activeProject}/managedZones/${zoneName}/rrsets`);
    if (res.ok) setRrsets((await res.json()).rrsets || []);
  }, [activeProject]);

  useEffect(() => { loadNetworks(); loadFirewalls(); loadZones(); }, [loadNetworks, loadFirewalls, loadZones]);
  useEffect(() => { if (selectedZone) loadRRSets(selectedZone.name); }, [selectedZone, loadRRSets]);

  // ── VPC actions ──────────────────────────────────────────────────────────────
  const createVPC = async () => {
    if (!vpcName.trim()) return;
    const res = await fetch(`/api/manage/network/projects/${activeProject}/global/networks`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: vpcName.trim(), autoCreateSubnetworks: autoSubnet }),
    });
    if (res.ok) { showToast(`VPC "${vpcName}" created`); setVpcName(''); loadNetworks(); }
    else showToast('Create failed', 'error');
  };

  const deleteVPC = async (name: string) => {
    await fetch(`/api/manage/network/projects/${activeProject}/global/networks/${name}`, { method: 'DELETE' });
    showToast(`VPC "${name}" deleted`); loadNetworks();
  };

  // ── Firewall actions ─────────────────────────────────────────────────────────
  const createFirewall = async () => {
    if (!fwName.trim()) return;
    const rule = fwAction === 'allow'
      ? { allowed: [{ IPProtocol: fwProtocol, ports: fwPorts ? fwPorts.split(',').map(p => p.trim()) : [] }] }
      : { denied:  [{ IPProtocol: fwProtocol, ports: fwPorts ? fwPorts.split(',').map(p => p.trim()) : [] }] };
    const body: any = {
      name: fwName.trim(),
      direction: fwDirection,
      ...rule,
    };
    if (fwDirection === 'INGRESS') body.sourceRanges = [fwSource];
    else body.destinationRanges = [fwSource];

    const res = await fetch(`/api/manage/network/projects/${activeProject}/global/firewalls`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (res.ok) { showToast(`Firewall "${fwName}" created`); setFwName(''); loadFirewalls(); }
    else showToast('Create failed', 'error');
  };

  const deleteFirewall = async (name: string) => {
    await fetch(`/api/manage/network/projects/${activeProject}/global/firewalls/${name}`, { method: 'DELETE' });
    showToast(`Firewall "${name}" deleted`); loadFirewalls();
  };

  // ── DNS zone actions ──────────────────────────────────────────────────────────
  const createZone = async () => {
    if (!newZoneName.trim() || !newDnsName.trim()) return;
    const res = await fetch(`/api/manage/dns/projects/${activeProject}/managedZones`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: newZoneName, dnsName: newDnsName, visibility: 'public' }),
    });
    if (res.ok) { showToast(`Zone "${newZoneName}" created`); setNewZoneName(''); setNewDnsName(''); loadZones(); }
    else showToast('Create failed', 'error');
  };

  const deleteZone = async (name: string) => {
    const res = await fetch(`/api/manage/dns/projects/${activeProject}/managedZones/${name}`, { method: 'DELETE' });
    if (res.status === 204 || res.ok) { showToast(`Zone "${name}" deleted`); setSelectedZone(null); loadZones(); }
    else { const e = await res.json(); showToast(e?.error?.message || 'Delete failed', 'error'); }
  };

  const addRecord = async () => {
    if (!selectedZone || !newRRName || !newRRData) return;
    const res = await fetch(`/api/manage/dns/projects/${activeProject}/managedZones/${selectedZone.name}/rrsets`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: newRRName, type: newRRType, ttl: parseInt(newRRTtl), rrdatas: [newRRData] }),
    });
    if (res.ok) { showToast('Record added'); setNewRRName(''); setNewRRData(''); loadRRSets(selectedZone.name); }
    else { const e = await res.json(); showToast(e?.error?.message || 'Failed', 'error'); }
  };

  const deleteRecord = async (name: string, type: string) => {
    if (!selectedZone) return;
    await fetch(`/api/manage/dns/projects/${activeProject}/managedZones/${selectedZone.name}/rrsets/${encodeURIComponent(name)}/${type}`, { method: 'DELETE' });
    showToast('Record deleted'); loadRRSets(selectedZone.name);
  };

  const rrColor = (t: string): any => ({ A: 'primary', AAAA: 'info', CNAME: 'secondary', MX: 'warning', TXT: 'success', NS: 'error' }[t] || 'default');

  return (
    <Box sx={{ animation: 'fadeIn 0.4s ease-out' }}>
      {/* Header */}
      <Box sx={{ mb: 4 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 1 }}>
          <HubIcon sx={{ color: '#1a73e8', fontSize: 28 }} />
          <Typography variant="h4" sx={{ fontWeight: 500 }}>Networking</Typography>
        </Box>
        <Typography variant="body1" sx={{ color: '#5f6368' }}>
          Manage VPC networks, firewall rules, and DNS zones for project{' '}
          <Chip label={activeProject} size="small" sx={{ ml: 0.5, backgroundColor: '#e8f0fe', color: '#1a73e8', fontWeight: 600 }} />
        </Typography>
      </Box>

      <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 3, borderBottom: '1px solid #dadce0' }}>
        <Tab icon={<LanIcon fontSize="small" />} label="VPC Networks" iconPosition="start" />
        <Tab icon={<SecurityIcon fontSize="small" />} label="Firewall Rules" iconPosition="start" />
        <Tab icon={<DnsIcon fontSize="small" />} label="Cloud DNS" iconPosition="start" />
      </Tabs>

      {/* ── TAB 0: VPC Networks ─────────────────────────────────────────────── */}
      {tab === 0 && (
        <Box>
          <Typography variant="h6" sx={{ mb: 2 }}>VPC Networks</Typography>
          <Box sx={{ display: 'flex', gap: 1.5, mb: 3, alignItems: 'flex-end' }}>
            <TextField size="small" label="Network Name" placeholder="my-vpc" value={vpcName} onChange={e => setVpcName(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))} sx={{ flex: 1 }} />
            <FormControl size="small" sx={{ minWidth: 180 }}>
              <InputLabel>Subnet Mode</InputLabel>
              <Select value={autoSubnet ? 'auto' : 'custom'} label="Subnet Mode" onChange={e => setAutoSubnet(e.target.value === 'auto')}>
                <MenuItem value="auto">Auto-create subnets</MenuItem>
                <MenuItem value="custom">Custom subnets</MenuItem>
              </Select>
            </FormControl>
            <Button variant="contained" startIcon={<AddIcon />} onClick={createVPC} disabled={!vpcName.trim()}>Create VPC</Button>
          </Box>
          <Table size="small">
            <TableHead><TableRow><TableCell>Name</TableCell><TableCell>Subnets</TableCell><TableCell>Created</TableCell><TableCell align="right">Actions</TableCell></TableRow></TableHead>
            <TableBody>
              {networks.length === 0 && <TableRow><TableCell colSpan={4} align="center" sx={{ color: '#80868b', py: 4 }}>No VPC networks yet.</TableCell></TableRow>}
              {networks.map(n => (
                <TableRow key={n.name} hover>
                  <TableCell sx={{ fontWeight: 600 }}>{n.name}</TableCell>
                  <TableCell><Chip size="small" label={n.autoCreateSubnetworks ? 'Auto' : 'Custom'} variant="outlined" /></TableCell>
                  <TableCell sx={{ color: '#80868b', fontSize: '0.8rem' }}>{new Date(n.creationTimestamp).toLocaleDateString()}</TableCell>
                  <TableCell align="right"><IconButton size="small" color="error" onClick={() => deleteVPC(n.name)}><DeleteIcon fontSize="small" /></IconButton></TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Box>
      )}

      {/* ── TAB 1: Firewall Rules ────────────────────────────────────────────── */}
      {tab === 1 && (
        <Box>
          <Typography variant="h6" sx={{ mb: 2 }}>Firewall Rules</Typography>
          <Box sx={{ display: 'flex', gap: 1.5, mb: 3, flexWrap: 'wrap', alignItems: 'flex-end', p: 2, bgcolor: '#f8f9fa', borderRadius: 2, border: '1px solid #dadce0' }}>
            <TextField size="small" label="Rule Name" placeholder="allow-http" value={fwName} onChange={e => setFwName(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))} sx={{ flex: 1, minWidth: 140 }} />
            <FormControl size="small" sx={{ minWidth: 100 }}>
              <InputLabel>Direction</InputLabel>
              <Select value={fwDirection} label="Direction" onChange={e => setFwDirection(e.target.value)}>
                <MenuItem value="INGRESS">INGRESS</MenuItem>
                <MenuItem value="EGRESS">EGRESS</MenuItem>
              </Select>
            </FormControl>
            <FormControl size="small" sx={{ minWidth: 90 }}>
              <InputLabel>Action</InputLabel>
              <Select value={fwAction} label="Action" onChange={e => setFwAction(e.target.value)}>
                <MenuItem value="allow">Allow</MenuItem>
                <MenuItem value="deny">Deny</MenuItem>
              </Select>
            </FormControl>
            <FormControl size="small" sx={{ minWidth: 90 }}>
              <InputLabel>Protocol</InputLabel>
              <Select value={fwProtocol} label="Protocol" onChange={e => setFwProtocol(e.target.value)}>
                {['tcp', 'udp', 'icmp', 'all'].map(p => <MenuItem key={p} value={p}>{p.toUpperCase()}</MenuItem>)}
              </Select>
            </FormControl>
            <TextField size="small" label="Ports" placeholder="80,443" value={fwPorts} onChange={e => setFwPorts(e.target.value)} sx={{ width: 100 }} disabled={fwProtocol === 'icmp' || fwProtocol === 'all'} />
            <TextField size="small" label={fwDirection === 'INGRESS' ? 'Source Range' : 'Dest Range'} placeholder="0.0.0.0/0" value={fwSource} onChange={e => setFwSource(e.target.value)} sx={{ minWidth: 130 }} />
            <Button variant="contained" startIcon={<AddIcon />} onClick={createFirewall} disabled={!fwName.trim()}>Add Rule</Button>
          </Box>
          <Table size="small">
            <TableHead><TableRow><TableCell>Name</TableCell><TableCell>Direction</TableCell><TableCell>Action</TableCell><TableCell>Protocol / Ports</TableCell><TableCell>Ranges</TableCell><TableCell>Priority</TableCell><TableCell align="right">Delete</TableCell></TableRow></TableHead>
            <TableBody>
              {firewalls.length === 0 && <TableRow><TableCell colSpan={7} align="center" sx={{ color: '#80868b', py: 4 }}>No firewall rules yet.</TableCell></TableRow>}
              {firewalls.map(fw => {
                const rules = fw.allowed?.length > 0 ? fw.allowed : fw.denied || [];
                const action = fw.allowed?.length > 0 ? 'allow' : 'deny';
                return (
                  <TableRow key={fw.name} hover>
                    <TableCell sx={{ fontWeight: 600 }}>{fw.name}</TableCell>
                    <TableCell><Chip size="small" label={fw.direction} variant="outlined" /></TableCell>
                    <TableCell><Chip size="small" label={action.toUpperCase()} color={action === 'allow' ? 'success' : 'error'} /></TableCell>
                    <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                      {rules.map(r => `${r.IPProtocol}${r.ports?.length ? ':' + r.ports.join(',') : ''}`).join(', ') || 'all'}
                    </TableCell>
                    <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.78rem', color: '#5f6368' }}>
                      {[...(fw.sourceRanges || []), ...(fw.destinationRanges || [])].join(', ')}
                    </TableCell>
                    <TableCell>{fw.priority}</TableCell>
                    <TableCell align="right"><IconButton size="small" color="error" onClick={() => deleteFirewall(fw.name)}><DeleteIcon fontSize="small" /></IconButton></TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </Box>
      )}

      {/* ── TAB 2: Cloud DNS ──────────────────────────────────────────────────── */}
      {tab === 2 && (
        <Box>
          {selectedZone ? (
            <>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 3 }}>
                <Button size="small" variant="outlined" onClick={() => setSelectedZone(null)}>← Back to Zones</Button>
                <Typography variant="h6">{selectedZone.name}</Typography>
                <Chip label={selectedZone.dnsName} size="small" sx={{ ml: 1 }} />
              </Box>
              <Box sx={{ display: 'flex', gap: 1, mb: 3, flexWrap: 'wrap', alignItems: 'flex-end', p: 2, bgcolor: '#f8f9fa', borderRadius: 2, border: '1px solid #dadce0' }}>
                <TextField size="small" label="Record Name (FQDN)" placeholder="www.example.com." value={newRRName} onChange={e => setNewRRName(e.target.value)} sx={{ flex: 2, minWidth: 200 }} />
                <FormControl size="small" sx={{ minWidth: 90 }}>
                  <InputLabel>Type</InputLabel>
                  <Select value={newRRType} label="Type" onChange={e => setNewRRType(e.target.value)}>
                    {['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'PTR', 'SRV'].map(t => <MenuItem key={t} value={t}>{t}</MenuItem>)}
                  </Select>
                </FormControl>
                <TextField size="small" label="TTL (s)" value={newRRTtl} onChange={e => setNewRRTtl(e.target.value)} sx={{ width: 80 }} />
                <TextField size="small" label="Data" placeholder="1.2.3.4" value={newRRData} onChange={e => setNewRRData(e.target.value)} sx={{ flex: 2, minWidth: 150 }} />
                <Button variant="contained" startIcon={<AddIcon />} onClick={addRecord} disabled={!newRRName || !newRRData}>Add Record</Button>
              </Box>
              <Table size="small">
                <TableHead><TableRow><TableCell>Name</TableCell><TableCell>Type</TableCell><TableCell>TTL</TableCell><TableCell>Data</TableCell><TableCell align="right">Delete</TableCell></TableRow></TableHead>
                <TableBody>
                  {rrsets.map(rr => (
                    <TableRow key={rr.name + rr.type} hover>
                      <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>{rr.name}</TableCell>
                      <TableCell><Chip label={rr.type} size="small" color={rrColor(rr.type)} /></TableCell>
                      <TableCell sx={{ color: '#80868b' }}>{rr.ttl}s</TableCell>
                      <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>{rr.rrdatas?.join(', ')}</TableCell>
                      <TableCell align="right">
                        {rr.type !== 'SOA' && rr.type !== 'NS' && (
                          <IconButton size="small" color="error" onClick={() => deleteRecord(rr.name, rr.type)}><DeleteIcon fontSize="small" /></IconButton>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </>
          ) : (
            <>
              <Typography variant="h6" sx={{ mb: 2 }}>Managed DNS Zones</Typography>
              <Box sx={{ display: 'flex', gap: 1.5, mb: 3, alignItems: 'flex-end' }}>
                <TextField size="small" label="Zone Name" placeholder="my-zone" value={newZoneName} onChange={e => setNewZoneName(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))} sx={{ flex: 1 }} />
                <TextField size="small" label="DNS Name" placeholder="example.com." value={newDnsName} onChange={e => setNewDnsName(e.target.value)} sx={{ flex: 1 }} />
                <Button variant="contained" startIcon={<AddIcon />} onClick={createZone} disabled={!newZoneName || !newDnsName}>Create Zone</Button>
              </Box>
              <Divider sx={{ mb: 2 }} />
              <Table size="small">
                <TableHead><TableRow><TableCell>Zone Name</TableCell><TableCell>DNS Name</TableCell><TableCell>Visibility</TableCell><TableCell>Name Servers</TableCell><TableCell align="right">Delete</TableCell></TableRow></TableHead>
                <TableBody>
                  {zones.length === 0 && <TableRow><TableCell colSpan={5} align="center" sx={{ color: '#80868b', py: 4 }}>No DNS zones yet.</TableCell></TableRow>}
                  {zones.map(z => (
                    <TableRow key={z.name} hover sx={{ cursor: 'pointer' }} onClick={() => setSelectedZone(z)}>
                      <TableCell sx={{ fontWeight: 600, color: '#1a73e8' }}>{z.name}</TableCell>
                      <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>{z.dnsName}</TableCell>
                      <TableCell><Chip size="small" label={z.visibility} color={z.visibility === 'public' ? 'success' : 'default'} variant="outlined" /></TableCell>
                      <TableCell sx={{ color: '#80868b', fontSize: '0.75rem' }}>{z.nameServers?.[0]}</TableCell>
                      <TableCell align="right">
                        <IconButton size="small" color="error" onClick={e => { e.stopPropagation(); deleteZone(z.name); }}><DeleteIcon fontSize="small" /></IconButton>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </>
          )}
        </Box>
      )}

      <Snackbar open={toast.open} autoHideDuration={3000} onClose={() => setToast(t => ({ ...t, open: false }))} anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}>
        <Alert severity={toast.severity} sx={{ width: '100%' }}>{toast.msg}</Alert>
      </Snackbar>

      <style>{`@keyframes fadeIn { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: none; } }`}</style>
    </Box>
  );
}
