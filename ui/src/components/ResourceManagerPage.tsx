import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert, Box, Button, Chip, CircularProgress, Dialog, DialogActions, DialogContent,
  DialogTitle, IconButton, MenuItem, Paper, Snackbar, TextField, Tooltip, Typography,
} from '@mui/material';
import AccountTreeIcon from '@mui/icons-material/AccountTree';
import CreateNewFolderIcon from '@mui/icons-material/CreateNewFolder';
import FolderIcon from '@mui/icons-material/Folder';
import InventoryIcon from '@mui/icons-material/Inventory2';
import DeleteOutlineIcon from '@mui/icons-material/Delete';
import DriveFileMoveIcon from '@mui/icons-material/DriveFileMove';
import RefreshIcon from '@mui/icons-material/Refresh';
import SecurityIcon from '@mui/icons-material/Security';

const API = '/api/manage/resourcemanager';

interface Folder {
  name: string;
  parent: string;
  displayName: string;
  state: string;
}

interface Project {
  projectId: string;
  projectNumber: string;
  name?: string;
  lifecycleState: string;
  parent?: { type: string; id: string };
}

interface Organization {
  name: string;
  displayName: string;
}

interface GrantedRole {
  role: string;
  grantedOn: string;
  inherited: boolean;
  roleKnown: boolean;
}

/** A node in the tree, with its depth so the row can be indented. */
interface Row {
  key: string;
  kind: 'organization' | 'folder' | 'project';
  label: string;
  name: string;
  depth: number;
  state?: string;
}

export default function ResourceManagerPage() {
  const [org, setOrg] = useState<Organization | null>(null);
  const [folders, setFolders] = useState<Folder[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  const [newFolder, setNewFolder] = useState({ open: false, displayName: '', parent: '' });
  const [newProject, setNewProject] = useState({ open: false, projectId: '', displayName: '', parent: '' });
  const [moveFolder, setMoveFolder] = useState({ open: false, name: '', label: '', destination: '' });
  const [policy, setPolicy] = useState<{ open: boolean; resource: string; roles: GrantedRole[]; principal: string }>({
    open: false, resource: '', roles: [], principal: '',
  });

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const orgs = await fetch(`${API}/v3/organizations:search`).then(r => r.json());
      const organization: Organization | undefined = orgs.organizations?.[0];
      if (!organization) {
        throw new Error('No organization is present. Start MiniSky and it will seed one.');
      }
      setOrg(organization);

      // folders.list is scoped to one parent, so the tree is walked level by level.
      const collected: Folder[] = [];
      const queue = [organization.name];
      while (queue.length) {
        const parent = queue.shift() as string;
        const page = await fetch(`${API}/v3/folders?parent=${encodeURIComponent(parent)}`).then(r => r.json());
        for (const folder of page.folders ?? []) {
          collected.push(folder);
          queue.push(folder.name);
        }
      }
      setFolders(collected);

      const projectPage = await fetch(`${API}/v1/projects`).then(r => r.json());
      setProjects(projectPage.projects ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  /** Flattens the hierarchy into indented rows, parents before children. */
  const rows = useMemo<Row[]>(() => {
    if (!org) return [];
    const out: Row[] = [{ key: org.name, kind: 'organization', label: org.displayName, name: org.name, depth: 0 }];

    const walk = (parent: string, depth: number) => {
      for (const folder of folders.filter(f => f.parent === parent)) {
        out.push({
          key: folder.name, kind: 'folder', label: folder.displayName,
          name: folder.name, depth, state: folder.state,
        });
        walk(folder.name, depth + 1);
        for (const project of projects.filter(p => p.parent && `${p.parent.type}s/${p.parent.id}` === folder.name)) {
          out.push({
            key: project.projectId, kind: 'project',
            label: project.name || project.projectId, name: `projects/${project.projectId}`,
            depth: depth + 1, state: project.lifecycleState,
          });
        }
      }
    };
    walk(org.name, 1);

    // Projects attached straight to the organization, or to nothing at all.
    for (const project of projects) {
      const parent = project.parent ? `${project.parent.type}s/${project.parent.id}` : '';
      if (parent === org.name || !parent) {
        out.push({
          key: project.projectId, kind: 'project',
          label: project.name || project.projectId, name: `projects/${project.projectId}`,
          depth: 1, state: project.lifecycleState,
        });
      }
    }
    return out;
  }, [org, folders, projects]);

  const parentChoices = useMemo(() => {
    const choices = org ? [{ value: org.name, label: `${org.displayName} (organization)` }] : [];
    return choices.concat(folders.map(f => ({ value: f.name, label: f.displayName })));
  }, [org, folders]);

  /** Reports a failure using the API's own message, which explains the rule that was broken. */
  const failed = async (res: Response) => {
    const body = await res.json().catch(() => null);
    return body?.error?.message || `request failed with ${res.status}`;
  };

  const createFolder = async () => {
    const res = await fetch(`${API}/v3/folders`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ displayName: newFolder.displayName, parent: newFolder.parent }),
    });
    if (!res.ok) { setError(await failed(res)); return; }
    setNewFolder({ open: false, displayName: '', parent: '' });
    setNotice(`Created folder ${newFolder.displayName}`);
    load();
  };

  const createProject = async () => {
    const res = await fetch(`${API}/v1/projects`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        projectId: newProject.projectId,
        name: newProject.displayName || newProject.projectId,
        parent: newProject.parent,
      }),
    });
    if (!res.ok) { setError(await failed(res)); return; }
    setNewProject({ open: false, projectId: '', displayName: '', parent: '' });
    setNotice(`Created project ${newProject.projectId}`);
    load();
  };

  const submitMove = async () => {
    const res = await fetch(`${API}/v3/${moveFolder.name}:move`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ destinationParent: moveFolder.destination }),
    });
    if (!res.ok) { setError(await failed(res)); return; }
    setMoveFolder({ open: false, name: '', label: '', destination: '' });
    setNotice(`Moved ${moveFolder.label}`);
    load();
  };

  const remove = async (row: Row) => {
    const path = row.kind === 'folder' ? `${API}/v3/${row.name}` : `${API}/v1/${row.name}`;
    const res = await fetch(path, { method: 'DELETE' });
    if (!res.ok) { setError(await failed(res)); return; }
    setNotice(`${row.label} marked for deletion — it can still be restored`);
    load();
  };

  const openPolicy = async (row: Row) => {
    setPolicy({ open: true, resource: row.name, roles: [], principal: '' });
  };

  const explain = async () => {
    const res = await fetch(`${API}/v1/internal/iam/analyze`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ principal: policy.principal, resource: policy.resource }),
    });
    if (!res.ok) { setError(await failed(res)); return; }
    const body = await res.json();
    setPolicy(p => ({ ...p, roles: body.roles ?? [] }));
  };

  const row = { display: 'flex', alignItems: 'center', gap: 1.5 };

  return (
    <Box sx={{ p: 4 }}>
      <Box sx={{ ...row, mb: 1 }}>
        <AccountTreeIcon sx={{ color: '#1a73e8' }} />
        <Typography variant="h5" sx={{ fontWeight: 500, color: '#3c4043' }}>Resource Manager</Typography>
        <Box sx={{ flexGrow: 1 }} />
        <Button startIcon={<CreateNewFolderIcon />} onClick={() => setNewFolder({ open: true, displayName: '', parent: org?.name ?? '' })}>
          New folder
        </Button>
        <Button startIcon={<InventoryIcon />} onClick={() => setNewProject({ open: true, projectId: '', displayName: '', parent: org?.name ?? '' })}>
          New project
        </Button>
        <Tooltip title="Reload"><IconButton onClick={load}><RefreshIcon /></IconButton></Tooltip>
      </Box>

      <Typography variant="body2" sx={{ color: '#5f6368', mb: 3 }}>
        The organization, folders and projects every other resource hangs from. Access granted here
        is inherited by everything beneath it.
      </Typography>

      {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError('')}>{error}</Alert>}

      <Paper variant="outlined" sx={{ borderRadius: 2 }}>
        {loading && <Box sx={{ p: 6, textAlign: 'center' }}><CircularProgress size={28} /></Box>}
        {!loading && rows.length === 0 && (
          <Box sx={{ p: 6, textAlign: 'center', color: '#5f6368' }}>Nothing here yet.</Box>
        )}
        {!loading && rows.map(r => (
          <Box
            key={`${r.kind}-${r.key}`}
            sx={{
              ...row,
              px: 2, py: 1.25, borderBottom: '1px solid #f1f3f4',
              pl: 2 + r.depth * 3.5,
              '&:hover': { backgroundColor: '#f8f9fa' },
              '&:hover .rm-actions': { opacity: 1 },
            }}
          >
            {r.kind === 'organization' && <AccountTreeIcon sx={{ color: '#5f6368', fontSize: 20 }} />}
            {r.kind === 'folder' && <FolderIcon sx={{ color: '#f9ab00', fontSize: 20 }} />}
            {r.kind === 'project' && <InventoryIcon sx={{ color: '#1a73e8', fontSize: 20 }} />}

            <Typography sx={{ fontWeight: r.kind === 'organization' ? 500 : 400, color: '#3c4043' }}>
              {r.label}
            </Typography>
            <Typography variant="caption" sx={{ color: '#9aa0a6', fontFamily: 'monospace' }}>
              {r.name}
            </Typography>
            {r.state && r.state !== 'ACTIVE' && (
              <Chip size="small" color="warning" label={r.state} sx={{ height: 20, fontSize: '0.7rem' }} />
            )}

            <Box sx={{ flexGrow: 1 }} />
            <Box className="rm-actions" sx={{ display: 'flex', gap: 0.5, opacity: 0, transition: 'opacity .15s' }}>
              <Tooltip title="Who has access here">
                <IconButton size="small" onClick={() => openPolicy(r)}><SecurityIcon fontSize="small" /></IconButton>
              </Tooltip>
              {r.kind === 'folder' && (
                <Tooltip title="Move to another parent">
                  <IconButton size="small" onClick={() => setMoveFolder({ open: true, name: r.name, label: r.label, destination: org?.name ?? '' })}>
                    <DriveFileMoveIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
              )}
              {r.kind !== 'organization' && (
                <Tooltip title="Delete (recoverable)">
                  <IconButton size="small" onClick={() => remove(r)}><DeleteOutlineIcon fontSize="small" /></IconButton>
                </Tooltip>
              )}
            </Box>
          </Box>
        ))}
      </Paper>

      <Dialog open={newFolder.open} onClose={() => setNewFolder({ ...newFolder, open: false })} fullWidth maxWidth="xs">
        <DialogTitle>New folder</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus fullWidth margin="dense" label="Display name"
            helperText="3-30 characters. Must be unique among its siblings."
            value={newFolder.displayName}
            onChange={e => setNewFolder({ ...newFolder, displayName: e.target.value })}
          />
          <TextField
            select fullWidth margin="dense" label="Parent"
            value={newFolder.parent}
            onChange={e => setNewFolder({ ...newFolder, parent: e.target.value })}
          >
            {parentChoices.map(c => <MenuItem key={c.value} value={c.value}>{c.label}</MenuItem>)}
          </TextField>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setNewFolder({ ...newFolder, open: false })}>Cancel</Button>
          <Button variant="contained" onClick={createFolder} disabled={!newFolder.displayName || !newFolder.parent}>Create</Button>
        </DialogActions>
      </Dialog>

      <Dialog open={newProject.open} onClose={() => setNewProject({ ...newProject, open: false })} fullWidth maxWidth="xs">
        <DialogTitle>New project</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus fullWidth margin="dense" label="Project ID"
            helperText="6-30 lowercase letters, digits and hyphens, starting with a letter."
            value={newProject.projectId}
            onChange={e => setNewProject({ ...newProject, projectId: e.target.value })}
          />
          <TextField
            fullWidth margin="dense" label="Display name (optional)"
            value={newProject.displayName}
            onChange={e => setNewProject({ ...newProject, displayName: e.target.value })}
          />
          <TextField
            select fullWidth margin="dense" label="Parent"
            value={newProject.parent}
            onChange={e => setNewProject({ ...newProject, parent: e.target.value })}
          >
            {parentChoices.map(c => <MenuItem key={c.value} value={c.value}>{c.label}</MenuItem>)}
          </TextField>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setNewProject({ ...newProject, open: false })}>Cancel</Button>
          <Button variant="contained" onClick={createProject} disabled={!newProject.projectId}>Create</Button>
        </DialogActions>
      </Dialog>

      <Dialog open={moveFolder.open} onClose={() => setMoveFolder({ ...moveFolder, open: false })} fullWidth maxWidth="xs">
        <DialogTitle>Move {moveFolder.label}</DialogTitle>
        <DialogContent>
          <TextField
            select fullWidth margin="dense" label="New parent"
            value={moveFolder.destination}
            onChange={e => setMoveFolder({ ...moveFolder, destination: e.target.value })}
          >
            {parentChoices.filter(c => c.value !== moveFolder.name).map(c => (
              <MenuItem key={c.value} value={c.value}>{c.label}</MenuItem>
            ))}
          </TextField>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setMoveFolder({ ...moveFolder, open: false })}>Cancel</Button>
          <Button variant="contained" onClick={submitMove} disabled={!moveFolder.destination}>Move</Button>
        </DialogActions>
      </Dialog>

      <Dialog open={policy.open} onClose={() => setPolicy({ ...policy, open: false })} fullWidth maxWidth="sm">
        <DialogTitle>Access on {policy.resource}</DialogTitle>
        <DialogContent>
          <Typography variant="body2" sx={{ color: '#5f6368', mb: 2 }}>
            Shows the roles a principal holds here, including those inherited from a folder or the
            organization above. MiniSky records policies but does not enforce them, so this
            describes the configuration rather than whether a request would be allowed.
          </Typography>
          <Box sx={{ display: 'flex', gap: 1 }}>
            <TextField
              fullWidth size="small" label="Principal"
              placeholder="user:ada@example.com"
              value={policy.principal}
              onChange={e => setPolicy({ ...policy, principal: e.target.value })}
              onKeyDown={e => { if (e.key === 'Enter') explain(); }}
            />
            <Button variant="contained" onClick={explain} disabled={!policy.principal}>Check</Button>
          </Box>

          <Box sx={{ mt: 2 }}>
            {policy.roles.length === 0 && (
              <Typography variant="body2" sx={{ color: '#9aa0a6' }}>
                No roles found for that principal here.
              </Typography>
            )}
            {policy.roles.map(gr => (
              <Box key={`${gr.role}-${gr.grantedOn}`} sx={{ ...row, py: 0.75 }}>
                <Typography sx={{ fontFamily: 'monospace', fontSize: '0.85rem' }}>{gr.role}</Typography>
                <Chip
                  size="small"
                  label={gr.inherited ? `inherited from ${gr.grantedOn}` : 'granted here'}
                  sx={{ height: 20, fontSize: '0.7rem' }}
                />
                {!gr.roleKnown && (
                  <Tooltip title="Not in MiniSky's curated catalogue, so its permissions are unknown">
                    <Chip size="small" color="warning" label="unknown role" sx={{ height: 20, fontSize: '0.7rem' }} />
                  </Tooltip>
                )}
              </Box>
            ))}
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPolicy({ ...policy, open: false })}>Close</Button>
        </DialogActions>
      </Dialog>

      <Snackbar
        open={!!notice} autoHideDuration={4000} onClose={() => setNotice('')}
        message={notice} anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      />
    </Box>
  );
}
