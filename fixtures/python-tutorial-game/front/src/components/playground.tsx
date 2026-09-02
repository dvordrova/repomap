import React, { useCallback, useState } from "react";
import styled from "styled-components";
import { Row, Col, Button, Spin, Card, Progress } from "antd";

import {
  ILevel,
  RequestState,
  SimulationRunResult,
  ISimulationStep,
} from "../utils/model";

import SimulationField from "./simulation_field";
import Editor from "./editor";
import { runLevel } from "../service/http";

const StyledButton = styled(Button)`
  margin-top: 10px;
  display: inline;
`;

const StyledSimulationColumn = styled(Col)`
  padding: 0px 10px 0px 10px;
`;

const StyledEditorColumn = styled(Col)`
  padding: 10px 10px 0px 10px;
  @media (min-width: 992px) {
    padding-top: 82.5px;
    padding-left: 0px;
  }
`;

interface IPlayGroundProps {
  level?: ILevel;
}

export default function PlayGround({ level }: IPlayGroundProps) {
  const [loadingRun, setLoadingRun] = useState(false);
  const [errorReason, setErrorReason] = useState<string>();
  const [code, setCode] = useState(
    localStorage.getItem(`code-${level?.id}`) || "",
  );
  const [codeChanged, setCodeChanged] = useState(true);
  const [simulationRunResult, setSimulationRunResult] =
    useState<SimulationRunResult>();
  const [runRequestFailed, setRunRequestFailed] = useState(false);
  const [simulationSteps, setSimulationSteps] = useState<ISimulationStep[]>();
  const [toggleSimulation, setToggleSimulation] = useState(false);
  const [simulationWasRun, setSimulationWasRun] = useState(false);

  const changeCode = useCallback(
    (newCode: string) => {
      setCode(newCode);
      setCodeChanged(true);
      localStorage.setItem(`code-${level?.id}`, newCode);
    },
    [level?.id],
  );

  const handleClick = useCallback(
    (_: any) => {
      if (level === undefined) {
        return;
      }
      if (!codeChanged) {
        setToggleSimulation(!toggleSimulation);
        return;
      }
      setLoadingRun(true);
      setCodeChanged(false);
      setSimulationSteps(undefined);
      runLevel({ level_id: level.id, code: code }).then((data) => {
        if (data.state === RequestState.Error) {
          setRunRequestFailed(true);
          setErrorReason(data.reason);
          setLoadingRun(false);
          return;
        }
        setRunRequestFailed(false);
        if (data.steps) {
          setSimulationSteps(data.steps);
          // for mobile
          window.scrollTo(0, 0);
        }
        setSimulationWasRun(true);
        setSimulationRunResult(data.result);
        setLoadingRun(false);
      });
    },
    [code, codeChanged, level, toggleSimulation],
  );

  return (
    <Row>
      <StyledSimulationColumn
        xs={{ flex: "100%" }}
        sm={{ flex: "100%" }}
        md={{ flex: "100%" }}
        lg={{ flex: "62%" }}
        xl={{ flex: "62%" }}
      >
        <SimulationField
          key={level?.id}
          level={level}
          simulationSteps={simulationSteps}
          toggleSimulation={toggleSimulation}
        />
      </StyledSimulationColumn>
      <StyledEditorColumn flex="auto">
        <Editor code={code} setCode={changeCode} />
        <Spin spinning={loadingRun}>
          <StyledButton
            type="primary"
            htmlType="submit"
            shape="round"
            size="large"
            onClick={handleClick}
            disabled={code === ""}
          >
            {simulationWasRun && codeChanged === false && !runRequestFailed
              ? "Рестарт"
              : "Старт"}
          </StyledButton>
          {runRequestFailed && !!errorReason && (
            <>
              <br />
              <Card title="Ошибка">
                <p>{errorReason}</p>
              </Card>
            </>
          )}
          {!runRequestFailed &&
            simulationRunResult === SimulationRunResult.Win && (
              <Progress percent={100} size="small" />
            )}
          {!runRequestFailed &&
            simulationRunResult === SimulationRunResult.Lose && (
              <Progress percent={70} size="small" status="exception" />
            )}
        </Spin>
      </StyledEditorColumn>
    </Row>
  );
}
