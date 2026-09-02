import React, { useEffect, useState } from "react";

import { useParams } from "react-router-dom";
import { ILevel } from "../utils/model";

import { getLevel } from "../service/http";
import { Rootpage } from "./root";
import PlayGround from "../components/playground";

export default function LevelPage() {
  let { levelIdFromPath } = useParams();
  const [level, setLevel] = useState<ILevel>();
  const levelId = parseInt(levelIdFromPath || "0");

  useEffect(() => {
    setLevel(undefined);
    if (levelId === undefined) {
      return;
    }
    getLevel(levelId).then((data) => {
      if (data.level) {
        setLevel(data.level);
      } else {
        console.error(data.state, data.reason);
      }
    });
  }, [levelId]);

  return (
    <Rootpage levelId={levelId}>
      <PlayGround level={level} key={level?.id} />
    </Rootpage>
  );
}
