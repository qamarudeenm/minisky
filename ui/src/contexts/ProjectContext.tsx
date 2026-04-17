import React, { createContext, useContext, useState, useEffect } from 'react';

type ProjectContextType = {
  activeProject: string;
  setActiveProject: (name: string) => void;
  availableProjects: string[];
  addProject: (name: string) => void;
};

const ProjectContext = createContext<ProjectContextType | undefined>(undefined);

export function ProjectProvider({ children }: { children: React.ReactNode }) {
  const [activeProject, setActiveProjectState] = useState<string>('local-dev-project');
  const [availableProjects, setAvailableProjects] = useState<string[]>(['local-dev-project']);

  useEffect(() => {
    const saved = localStorage.getItem('minisky-active-project');
    const savedList = localStorage.getItem('minisky-projects-list');
    
    if (saved) {
      setActiveProjectState(saved);
    }
    if (savedList) {
      try {
        setAvailableProjects(JSON.parse(savedList));
      } catch (e) {}
    }
  }, []);

  const setActiveProject = (name: string) => {
    setActiveProjectState(name);
    localStorage.setItem('minisky-active-project', name);
  };

  const addProject = (name: string) => {
    if (!availableProjects.includes(name)) {
      const newList = [...availableProjects, name];
      setAvailableProjects(newList);
      localStorage.setItem('minisky-projects-list', JSON.stringify(newList));
    }
    setActiveProject(name);
  };

  return (
    <ProjectContext.Provider value={{ activeProject, setActiveProject, availableProjects, addProject }}>
      {children}
    </ProjectContext.Provider>
  );
}

export function useProjectContext() {
  const context = useContext(ProjectContext);
  if (context === undefined) {
    throw new Error('useProjectContext must be used within a ProjectProvider');
  }
  return context;
}
