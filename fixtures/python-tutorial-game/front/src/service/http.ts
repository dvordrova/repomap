import axios from "axios";

import {
  IGetLevelsResponse,
  IRunDescription,
  IRunLevelRequest,
  ILevelDescription,
} from "../utils/model";

export async function getLevels(): Promise<IGetLevelsResponse> {
  try {
    const response = await axios.get<IGetLevelsResponse>("/api/levels");
    return response.data;
  } catch (error) {
    throw error;
  }
}

export async function getLevel(levelId: number): Promise<ILevelDescription> {
  try {
    const response = await axios.get<ILevelDescription>(
      `/api/level/${levelId}`,
    );
    return response.data;
  } catch (error) {
    throw error;
  }
}

export async function runLevel(
  request: IRunLevelRequest,
): Promise<IRunDescription> {
  try {
    const response = await axios.post<IRunDescription>(
      "/api/level/run",
      request,
    );
    return response.data;
  } catch (error) {
    throw error;
  }
}
