import axios from 'axios';

export interface UserProjects {
  username: string;
  projects: string[];
}

export class Projects {
  private static instance: Projects;
  private projects: string[] | null = null;

  public static getInstance(): Projects {
    if (!Projects.instance) {
      Projects.instance = new Projects();
    }
    return Projects.instance;
  }

  getProjects(): string[] | null {
    return this.projects;
  }

  private buildMiddlewareUrl(): string {
    const url = new URL(window.location.origin);
    url.hostname = url.hostname.replace(/\bhubble\b/, 'hubble-middleware');
    return `${url.origin}/projects`;
  }

  async setProjects(token: string): Promise<void> {
    const url = this.buildMiddlewareUrl();
    const res = await axios.get<UserProjects>(url, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!Array.isArray(res.data?.projects)) {
      throw new Error('Invalid response from projects API');
    }
    this.projects = res.data.projects;
  }
}
