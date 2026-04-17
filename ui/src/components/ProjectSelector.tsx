import { useState } from 'react';
import { Box, Typography, Menu, MenuItem, Button, Divider, TextField } from '@mui/material';
import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown';
import AccountTreeIcon from '@mui/icons-material/AccountTree';
import { useProjectContext } from '../contexts/ProjectContext';

export default function ProjectSelector() {
  const { activeProject, setActiveProject, availableProjects, addProject } = useProjectContext();
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const [isCreating, setIsCreating] = useState(false);
  const [newProjectName, setNewProjectName] = useState('');

  const handleClick = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
    setIsCreating(false);
    setNewProjectName('');
  };

  const handleSelect = (proj: string) => {
    setActiveProject(proj);
    handleClose();
  };

  const handleCreate = () => {
    if (newProjectName.trim()) {
      addProject(newProjectName.trim().toLowerCase().replace(/[^a-z0-9-]/g, ''));
      handleClose();
    }
  };

  return (
    <Box sx={{ position: 'absolute', top: 24, right: 48, zIndex: 10 }}>
      <Button 
        variant="text" 
        onClick={handleClick}
        sx={{ 
          color: '#5f6368', 
          textTransform: 'none', 
          fontWeight: 500,
          display: 'flex',
          alignItems: 'center',
          gap: 1
        }}
      >
        <AccountTreeIcon fontSize="small" sx={{ color: '#1a73e8' }} />
        <Typography variant="body2" sx={{ fontWeight: 600 }}>Project: </Typography>
        {activeProject}
        <ArrowDropDownIcon />
      </Button>

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleClose}
      >
        <Box sx={{ width: 280, mt: 1, p: 1 }}>
        <Typography variant="overline" sx={{ px: 2, color: '#80868b' }}>Select Isolated Context</Typography>
        
        {availableProjects.map((proj) => (
          <MenuItem 
            key={proj} 
            selected={proj === activeProject}
            onClick={() => handleSelect(proj)}
            sx={{ borderRadius: '4px', mb: 0.5 }}
          >
            {proj}
          </MenuItem>
        ))}

        <Divider sx={{ my: 1 }} />

        {isCreating ? (
          <Box sx={{ px: 2, pb: 1 }}>
            <TextField 
              autoFocus
              size="small" 
              fullWidth 
              placeholder="my-project-id"
              value={newProjectName}
              onChange={(e) => setNewProjectName(e.target.value)}
              sx={{ mb: 1 }}
            />
            <Box sx={{ display: 'flex', gap: 1 }}>
              <Button size="small" variant="contained" fullWidth onClick={handleCreate}>Create</Button>
              <Button size="small" variant="outlined" fullWidth onClick={() => setIsCreating(false)}>Cancel</Button>
            </Box>
          </Box>
        ) : (
          <MenuItem sx={{ borderRadius: '4px', color: '#1a73e8' }} onClick={() => setIsCreating(true)}>
            + New Project Context
          </MenuItem>
        )}
        </Box>
      </Menu>
    </Box>
  );
}
