import type { Me, Project, ProjectTemplate } from '../types'

// Boarding stays mounted until the person finishes the funnel, even after
// the first project exists. Done is stored per project id so a wiped hub
// (new prj-) shows the funnel again.

export function isHostedOperator(me: Me): boolean {
  return me.platformAdmin === true && me.offering === 'hosted'
}

export function orgNeedsName(me: Me): boolean {
  const name = me.orgs?.[0]?.name
  const unnamed = me.defaultOrgName || 'default'
  return Boolean(name && name === unnamed)
}

export const BOARDING_STORAGE_KEY = 'initagent.boarding'

export type BoardingStep = 'name' | 'template' | 'project' | 'repo' | 'worker' | 'task'

export type BoardingRecord = {
  done: true
  projectId: string
}

export function readBoardingRecord(): BoardingRecord | null {
  try {
    const raw = localStorage.getItem(BOARDING_STORAGE_KEY)
    if (!raw) return null
    if (raw === 'done') {
      return { done: true, projectId: '' }
    }
    const parsed = JSON.parse(raw) as { done?: unknown; projectId?: unknown }
    if (parsed?.done === true && typeof parsed.projectId === 'string') {
      return { done: true, projectId: parsed.projectId }
    }
  } catch {
    /* ignore a broken value */
  }
  return null
}

export function markBoardingDone(projectId: string): void {
  const record: BoardingRecord = { done: true, projectId }
  localStorage.setItem(BOARDING_STORAGE_KEY, JSON.stringify(record))
}

export function isBoardingComplete(projects: Project[]): boolean {
  const record = readBoardingRecord()
  if (!record || projects.length === 0) return false
  if (record.projectId === '') return true
  return projects.some((item) => item.id === record.projectId)
}

// nextStepAfterProject is the per-template fork: repo only when that
// template needs one and the row still has no remote.
export function nextStepAfterProject(
  project: Project,
  templates: ProjectTemplate[],
  workerJoined: boolean,
): BoardingStep {
  const tmpl = templates.find((item) => item.id === project.templateId)
  if (tmpl?.needsRepo && !project.repoRemote) return 'repo'
  return workerJoined ? 'task' : 'worker'
}

export function resumeBoardingStep(
  me: Me,
  project: Project | null,
  templates: ProjectTemplate[],
  workerJoined: boolean,
): BoardingStep {
  if (orgNeedsName(me)) return 'name'
  if (!project) return 'template'
  return nextStepAfterProject(project, templates, workerJoined)
}
