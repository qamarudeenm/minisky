import { useState, useEffect, useRef } from 'react';
import {
  Box, Typography, Chip, TextField, Select, MenuItem, FormControl,
  InputLabel, IconButton, Tooltip, CircularProgress,
  ToggleButtonGroup, ToggleButton
} from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import StopIcon from '@mui/icons-material/Stop';
import FilterListIcon from '@mui/icons-material/FilterList';
import TerminalIcon from '@mui/icons-material/Terminal';
import CloudIcon from '@mui/icons-material/Cloud';
import MemoryIcon from '@mui/icons-material/Memory';
import StorageIcon from '@mui/icons-material/Storage';
import CodeIcon from '@mui/icons-material/Code';

type LogEntry = {
  insertId: string;
  timestamp: string;
  severity: string;
  textPayload?: string;
  jsonPayload?: Record<string, unknown>;
  logName: string;
  resource?: { type: string; labels?: Record<string, string> };
};

type ContainerItem = { name: string; status: string; image: string };

const SEVERITY_COLOR: Record<string, string> = {
  DEBUG:     '#9aa0a6',
  INFO:      '#4fc3f7',
  NOTICE:    '#81c995',
  WARNING:   '#fbbc04',
  ERROR:     '#f28b82',
  CRITICAL:  '#ff6d00',
  ALERT:     '#ea4335',
  EMERGENCY: '#b00020',
};

const RESOURCE_ICON: Record<string, React.ReactNode> = {
  'cloud_function': <CodeIcon fontSize="small" />,
  'cloud_run_revision': <CloudIcon fontSize="small" />,
  'gce_instance': <MemoryIcon fontSize="small" />,
  'global': <StorageIcon fontSize="small" />,
};

function severityBg(s: string) {
  const c = SEVERITY_COLOR[s?.toUpperCase()] ?? '#9aa0a6';
  return c + '33'; // 20% alpha
}

export default function LogExplorer() {
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [containers, setContainers] = useState<ContainerItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [streaming, setStreaming] = useState(false);
  const [filterSeverity, setFilterSeverity] = useState('ALL');
  const [filterResource, setFilterResource] = useState('ALL');
  const [filterText, setFilterText] = useState('');
  const [viewMode, setViewMode] = useState<'centralized' | 'container'>('centralized');
  const [selectedContainer, setSelectedContainer] = useState('');
  const [containerLogs, setContainerLogs] = useState('');
  const [containerLoading, setContainerLoading] = useState(false);
  const streamTimer = useRef<ReturnType<typeof setInterval> | null>(null);
  const logEndRef = useRef<HTMLDivElement>(null);

  const fetchEntries = async () => {
    setLoading(true);
    try {
      const res = await fetch('/api/manage/logging/entries');
      if (res.ok) {
        const data = await res.json();
        setEntries((data.entries as LogEntry[]) ?? []);
      }
    } finally {
      setLoading(false);
    }
  };

  const fetchContainers = async () => {
    const res = await fetch('/api/manage/logging/container');
    if (res.ok) {
      const data = await res.json();
      setContainers(Array.isArray(data) ? data : []);
    }
  };

  const fetchContainerLogs = async (name: string) => {
    if (!name) return;
    setContainerLoading(true);
    try {
      const res = await fetch(`/api/manage/logging/container?name=${encodeURIComponent(name)}`);
      if (res.ok) setContainerLogs(await res.text());
      else setContainerLogs('No logs available.');
    } finally {
      setContainerLoading(false);
    }
  };

  useEffect(() => {
    fetchEntries();
    fetchContainers();
  }, []);

  // Auto-scroll to bottom when streaming
  useEffect(() => {
    if (streaming) logEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [entries, containerLogs, streaming]);

  const startStream = () => {
    setStreaming(true);
    streamTimer.current = setInterval(() => {
      if (viewMode === 'centralized') fetchEntries();
      else if (selectedContainer) fetchContainerLogs(selectedContainer);
    }, 2000);
  };

  const stopStream = () => {
    setStreaming(false);
    if (streamTimer.current) clearInterval(streamTimer.current);
  };

  useEffect(() => () => { if (streamTimer.current) clearInterval(streamTimer.current); }, []);

  const filteredEntries = [...entries].filter(e => {
    if (filterSeverity !== 'ALL' && e.severity?.toUpperCase() !== filterSeverity) return false;
    if (filterResource !== 'ALL' && e.resource?.type !== filterResource) return false;
    if (filterText) {
      const searchTerms = filterText.toLowerCase();
      const text = (e.textPayload ?? JSON.stringify(e.jsonPayload ?? '')).toLowerCase();
      const resName = (e.resource?.labels?.name ?? '').toLowerCase();
      if (!text.includes(searchTerms) && !resName.includes(searchTerms)) return false;
    }
    return true;
  }).sort((a, b) => {
    return new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime();
  });

  const defaultResources = ['cloud_function', 'cloud_run_revision', 'gce_instance', 'global'];
  const resourceTypes = ['ALL', ...Array.from(new Set([...defaultResources, ...entries.map(e => e.resource?.type ?? 'global')]))];

  return (
    <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column', background: '#0d1117', color: '#c9d1d9' }}>
      {/* Header */}
      <Box sx={{
        px: 3, py: 2,
        background: 'linear-gradient(135deg, #1a1f2e 0%, #161b27 100%)',
        borderBottom: '1px solid #30363d',
        display: 'flex', alignItems: 'center', gap: 2, flexWrap: 'wrap'
      }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
          <Box sx={{
            width: 36, height: 36, borderRadius: '10px',
            background: 'linear-gradient(135deg, #1e88e5, #0d47a1)',
            display: 'flex', alignItems: 'center', justifyContent: 'center'
          }}>
            <TerminalIcon sx={{ color: '#fff', fontSize: 18 }} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem', color: '#e6edf3', lineHeight: 1 }}>
              Cloud Logging
            </Typography>
            <Typography variant="caption" sx={{ color: '#8b949e' }}>
              Centralized log aggregation for all MiniSky services
            </Typography>
          </Box>
        </Box>

        <Box sx={{ flex: 1 }} />

        <ToggleButtonGroup
          value={viewMode}
          exclusive
          onChange={(_, v) => v && setViewMode(v)}
          size="small"
          sx={{
            '& .MuiToggleButton-root': {
              color: '#8b949e', borderColor: '#30363d', fontSize: '0.7rem',
              '&.Mui-selected': { background: '#1e88e520', color: '#90caf9', borderColor: '#1e88e5' }
            }
          }}
        >
          <ToggleButton value="centralized">Structured Logs</ToggleButton>
          <ToggleButton value="container">Container Output</ToggleButton>
        </ToggleButtonGroup>

        <Tooltip title={streaming ? 'Stop streaming' : 'Stream logs (2s refresh)'}>
          <IconButton size="small" onClick={streaming ? stopStream : startStream}
            sx={{ color: streaming ? '#f28b82' : '#81c995', border: '1px solid', borderColor: 'currentColor' }}>
            {streaming ? <StopIcon fontSize="small" /> : <PlayArrowIcon fontSize="small" />}
          </IconButton>
        </Tooltip>
        <Tooltip title="Refresh now">
          <IconButton size="small" onClick={() => { fetchEntries(); fetchContainers(); }}
            sx={{ color: '#8b949e' }}>
            <RefreshIcon fontSize="small" />
          </IconButton>
        </Tooltip>
      </Box>

      {/* Filter bar */}
      {viewMode === 'centralized' && (
        <Box sx={{
          px: 3, py: 1.5, display: 'flex', gap: 2, alignItems: 'center',
          background: '#161b27', borderBottom: '1px solid #21262d', flexWrap: 'wrap'
        }}>
          <FilterListIcon sx={{ color: '#8b949e', fontSize: 18 }} />
          <TextField
            size="small"
            placeholder="Filter logs..."
            value={filterText}
            onChange={e => setFilterText(e.target.value)}
            sx={{
              minWidth: 200,
              '& .MuiInputBase-root': { background: '#0d1117', color: '#c9d1d9', fontSize: '0.8rem' },
              '& .MuiOutlinedInput-notchedOutline': { borderColor: '#30363d' },
            }}
          />
          <FormControl size="small" sx={{ minWidth: 130 }}>
            <InputLabel sx={{ color: '#8b949e', fontSize: '0.8rem' }}>Severity</InputLabel>
            <Select value={filterSeverity} label="Severity" onChange={e => setFilterSeverity(e.target.value)}
              sx={{ color: '#c9d1d9', background: '#0d1117', fontSize: '0.8rem',
                '& .MuiOutlinedInput-notchedOutline': { borderColor: '#30363d' } }}>
              {['ALL','DEBUG','INFO','NOTICE','WARNING','ERROR','CRITICAL'].map(s => (
                <MenuItem key={s} value={s} sx={{ fontSize: '0.8rem' }}>{s}</MenuItem>
              ))}
            </Select>
          </FormControl>
          <FormControl size="small" sx={{ minWidth: 160 }}>
            <InputLabel sx={{ color: '#8b949e', fontSize: '0.8rem' }}>Resource</InputLabel>
            <Select value={filterResource} label="Resource" onChange={e => setFilterResource(e.target.value)}
              sx={{ color: '#c9d1d9', background: '#0d1117', fontSize: '0.8rem',
                '& .MuiOutlinedInput-notchedOutline': { borderColor: '#30363d' } }}>
              {resourceTypes.map(t => <MenuItem key={t} value={t} sx={{ fontSize: '0.8rem' }}>{t}</MenuItem>)}
            </Select>
          </FormControl>
          <Chip label={`${filteredEntries.length} entries`} size="small"
            sx={{ ml: 'auto', background: '#21262d', color: '#8b949e', fontSize: '0.7rem' }} />
          {streaming && (
            <Chip label="● LIVE" size="small" sx={{ background: '#81c99520', color: '#81c995', fontSize: '0.7rem',
              animation: 'pulse 2s infinite', '@keyframes pulse': { '0%,100%': { opacity: 1 }, '50%': { opacity: 0.5 } } }} />
          )}
        </Box>
      )}

      {/* Container selector bar */}
      {viewMode === 'container' && (
        <Box sx={{ px: 3, py: 1.5, background: '#161b27', borderBottom: '1px solid #21262d', display: 'flex', gap: 2, alignItems: 'center' }}>
          <FormControl size="small" sx={{ minWidth: 280 }}>
            <InputLabel sx={{ color: '#8b949e', fontSize: '0.8rem' }}>Select Container</InputLabel>
            <Select value={selectedContainer} label="Select Container"
              onChange={e => { setSelectedContainer(e.target.value); fetchContainerLogs(e.target.value); }}
              sx={{ color: '#c9d1d9', background: '#0d1117', fontSize: '0.8rem',
                '& .MuiOutlinedInput-notchedOutline': { borderColor: '#30363d' } }}>
              {containers.map(c => (
                <MenuItem key={c.name} value={c.name} sx={{ fontSize: '0.8rem' }}>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Box sx={{ width: 8, height: 8, borderRadius: '50%',
                      background: c.status.startsWith('Up') ? '#81c995' : '#f28b82' }} />
                    {c.name.replace('minisky-serverless-', '')}
                    <Chip label={c.status.startsWith('Up') ? 'Running' : 'Exited'} size="small"
                      sx={{ height: 16, fontSize: '0.6rem',
                        background: c.status.startsWith('Up') ? '#81c99520' : '#f28b8220',
                        color: c.status.startsWith('Up') ? '#81c995' : '#f28b82' }} />
                  </Box>
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          {selectedContainer && (
            <Tooltip title="Refresh container logs">
              <IconButton size="small" onClick={() => fetchContainerLogs(selectedContainer)} sx={{ color: '#8b949e' }}>
                <RefreshIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          )}
          {streaming && (
            <Chip label="● LIVE" size="small" sx={{ background: '#81c99520', color: '#81c995', fontSize: '0.7rem' }} />
          )}
        </Box>
      )}

      {/* Log Content */}
      <Box sx={{ flex: 1, overflow: 'auto', p: 0 }}>
        {viewMode === 'centralized' ? (
          loading && entries.length === 0 ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', flexDirection: 'column', gap: 2 }}>
              <CircularProgress size={32} sx={{ color: '#1e88e5' }} />
              <Typography variant="body2" sx={{ color: '#8b949e' }}>Fetching log entries...</Typography>
            </Box>
          ) : filteredEntries.length === 0 ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', flexDirection: 'column', gap: 2 }}>
              <TerminalIcon sx={{ fontSize: 48, color: '#21262d' }} />
              <Typography variant="body2" sx={{ color: '#8b949e' }}>No log entries yet.</Typography>
              <Typography variant="caption" sx={{ color: '#8b949e' }}>
                Deploy a function or service and logs will appear here automatically.
              </Typography>
            </Box>
          ) : (
            <Box sx={{ fontFamily: 'monospace' }}>
              {filteredEntries.map((entry, idx) => {
                const sev = (entry.severity ?? 'INFO').toUpperCase();
                const text = entry.textPayload ?? JSON.stringify(entry.jsonPayload ?? '', null, 2);
                const resType = entry.resource?.type ?? 'global';
                const resName = entry.resource?.labels?.name ?? entry.logName?.split('/logs/')[1] ?? '';
                return (
                  <Box key={entry.insertId ?? idx} sx={{
                    display: 'flex', alignItems: 'flex-start', gap: 1.5, px: 2, py: 0.75,
                    background: idx % 2 === 0 ? '#0d1117' : '#0a0e17',
                    borderLeft: `3px solid ${SEVERITY_COLOR[sev] ?? '#30363d'}`,
                    '&:hover': { background: '#161b27' },
                    transition: 'background 0.1s'
                  }}>
                    {/* Timestamp */}
                    <Typography variant="caption" sx={{ color: '#8b949e', minWidth: 175, pt: 0.25, fontSize: '0.72rem', fontFamily: 'monospace' }}>
                      {entry.timestamp ? new Date(entry.timestamp).toLocaleString() : '—'}
                    </Typography>
                    {/* Severity badge */}
                    <Box sx={{
                      minWidth: 72, px: 0.75, py: 0.1, borderRadius: '4px', textAlign: 'center',
                      background: severityBg(sev), border: `1px solid ${SEVERITY_COLOR[sev] ?? '#30363d'}33`
                    }}>
                      <Typography sx={{ fontSize: '0.65rem', fontWeight: 700, color: SEVERITY_COLOR[sev] ?? '#8b949e', letterSpacing: '0.05em' }}>
                        {sev}
                      </Typography>
                    </Box>
                    {/* Resource */}
                    <Tooltip title={resType}>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 130, color: '#8b949e' }}>
                        {RESOURCE_ICON[resType] ?? <CloudIcon fontSize="small" />}
                        <Typography variant="caption" sx={{ fontSize: '0.72rem', color: '#6e7681', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {resName || resType}
                        </Typography>
                      </Box>
                    </Tooltip>
                    {/* Message */}
                    <Typography sx={{ fontSize: '0.8rem', color: '#c9d1d9', flex: 1, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                      {text}
                    </Typography>
                  </Box>
                );
              })}
              <div ref={logEndRef} />
            </Box>
          )
        ) : (
          /* Container raw logs */
          !selectedContainer ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', flexDirection: 'column', gap: 2 }}>
              <TerminalIcon sx={{ fontSize: 48, color: '#21262d' }} />
              <Typography variant="body2" sx={{ color: '#8b949e' }}>Select a container above to view its output.</Typography>
            </Box>
          ) : containerLoading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%' }}>
              <CircularProgress size={28} sx={{ color: '#1e88e5' }} />
            </Box>
          ) : (
            <Box sx={{ p: 2, fontFamily: 'monospace', fontSize: '0.8rem', whiteSpace: 'pre-wrap', color: '#c9d1d9', lineHeight: 1.6 }}>
              {containerLogs || 'No output captured yet.'}
              <div ref={logEndRef} />
            </Box>
          )
        )}
      </Box>
    </Box>
  );
}
