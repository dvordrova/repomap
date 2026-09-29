// Every card represents one saved report item, including each original input.
export function groupInputs(inputs=[],translate=text=>text) {
  const kinds=[['request','Incoming requests'],['command','Commands'],['setting','Settings'],['background','Background work'],['interaction','User interactions'],['other','Other operations']];
  const groups=new Map(kinds.map(([kind,title])=>[kind,{kind,title:translate(title),inputs:[]}]));
  for(const input of inputs){
    const kind=['scheduled','continuous'].includes(input.activation)?'background':groups.has(input.activation)?input.activation:'other';
    groups.get(kind).inputs.push(input);
  }
  return [...groups.values()].filter(group=>group.inputs.length);
}

// A title breaks only between words, as the browser breaks it: closing
// punctuation stays with the word before it and an opening bracket with the
// word after it. Redis's input collection read "redis-server (executable"
// above a lone ")". A word breaks inside only when it alone is wider than the
// line: after a separator or between camelCase words when it can, and never
// so that a line starts with closing punctuation or ends with an opening one.
const closing=/^[)\]}»›”’,.;:!?…%]/u,opening=/[(\[{«‹“‘]$/u;
export function titleWords(text){
  const words=[];
  for(const word of String(text||'').split(/\s+/).filter(Boolean)){
    if(words.length&&(closing.test(word)||opening.test(words.at(-1))))words[words.length-1]+=' '+word;
    else words.push(word);
  }
  return words;
}
export function widestWord(text,font,measure){
  return Math.max(0,...titleWords(text).map(word=>measure(word,font)));
}
function breakWord(word,width,font,measure){
  const letters=Array.from(word),pieces=[];
  for(let start=0;start<letters.length;){
    let end=start+1;
    while(end<letters.length&&measure(letters.slice(start,end+1).join(''),font)<=width)end++;
    if(end<letters.length){
      for(let at=end;at>start+(end-start)/2;at--){
        const before=letters[at-1],after=letters[at];
        if(/[/_\-.:]/.test(before)||/[\p{Ll}\d]/u.test(before)&&/\p{Lu}/u.test(after)||after==='('){end=at;break;}
      }
      while(end>start+1&&(closing.test(letters[end])||opening.test(letters[end-1])))end--;
    }
    pieces.push(letters.slice(start,end).join(''));start=end;
  }
  return pieces;
}
export function wrapText(text,width,font,measure){
    const lines=[''];
    for(const word of titleWords(text)){
      const last=lines.length-1, candidate=lines[last]?`${lines[last]} ${word}`:word;
      if(measure(candidate,font)<=width){lines[last]=candidate;continue;}
      if(lines[last])lines.push('');
      const pieces=measure(word,font)<=width?[word]:breakWord(word,width,font,measure);
      lines[lines.length-1]=pieces[0];lines.push(...pieces.slice(1));
    }
    return lines;
}

// A title reads whole. When its widest word is wider than the room, its type
// shrinks until that word fits rather than breaking it: a destination frame
// fitted one pixel narrower than reserved read "DNS resolve" over a clipped
// "r".
export function fitTitle(title,width,font,measure){
  const widest=widestWord(title,font,measure),scale=widest>width?width/widest:1;
  return {scale,lines:wrapText(title,scale<1?widest:width,font,measure)};
}

// A program's Outside frame holds its destinations as chips of one size,
// each naming its destination in at most two lines, in rows wrapped toward
// a square instead of one strip (overview.mjs outsideChips): beside or
// under its program it takes little of the map's width.
// `top` is the band its title "Outside" stands in.
export const chip={width:112,height:40,gap:8,side:12,top:34};
export function chipGrid(count){
  let best=null;
  for(let columns=1;columns<=Math.max(1,count);columns++){
    const rows=Math.ceil(count/columns);
    const width=2*chip.side+columns*chip.width+(columns-1)*chip.gap,height=chip.top+chip.side+rows*chip.height+(rows-1)*chip.gap;
    const distance=Math.abs(Math.log(width/height));
    if(!best||distance<best.distance-1e-9)best={columns,rows,width,height,distance};
  }
  return best;
}

// A card as canvas.css draws it: 260px wide inside a 1.5px border and 16px
// padding, its title keeping 34px beside it for a part's zoom button. Text is
// wrapped at that real column. Wrapped at 228 and 194, a line between the
// column and those widths broke again in the browser: "Implements Redis set
// commands and" (225.84px in a 225px column) left "and" alone on a line.
export const card={width:260,border:1.5,padding:16,zoom:34};
export const cardText=card.width-2*card.border-2*card.padding;
// An input's kind mark before its name: 14px, a 6px gap (canvas.css) and
// the browser's rounding.
export const kindMark=22;

// A narrow external heading may need the full card width below its zoom mark.
// Measure the same heading for layout reservation and for the visible card.
export function overviewHeading(item,screenWidth,measure){
  const collection=['communication','inputs'].includes(item.branch);
  const size=collection?13:18,lineHeight=collection?17:23,font=`700 ${size}px system-ui`;
  const title=item.heading||item.name||item.title;
  const clearZoom=titleWords(title).some(word=>measure(word,font)>screenWidth-64);
  const width=Math.max(1,Math.min(304,screenWidth-(clearZoom?(collection?16:32):64)));
  const {scale,lines}=fitTitle(title,width,font,measure);
  return {width,clearZoom,lines,scale,fontSize:size*scale,lineHeight:lineHeight*scale,height:lines.length*lineHeight*scale+(clearZoom?32:0)};
}

// A whole-map fit that cannot give a summary its reserved room (a short window,
// a crowded map) draws the summary at that room and scales it down whole, the
// way a small group keeps a smaller complete label. Drawn into the short box,
// Redis's 1280×720 first screen cut "TCP endpoint" below its frame and
// "Background" out of its input list. The result is the largest scale at
// which the summary's minimum width and measured height fit its screen box.
export function overviewScale(item,screenWidth,screenHeight,availableHeight=Infinity){
  const fits=scale=>screenWidth/scale+1e-6>=(item.overviewMinWidth||0)&&
    screenHeight/scale+1e-6>=(item.overviewHeightAtWidth?.(screenWidth/scale,{availableHeight:availableHeight/scale})||0);
  if(fits(1))return 1;
  let low=0,high=1;
  for(let i=0;i<24;i++){const middle=(low+high)/2;if(fits(middle))low=middle;else high=middle;}
  return Math.max(low,1e-3);
}

// A group's fixed world box can be much smaller than its siblings at the
// common reveal threshold. Fit its complete name once, never hide it or
// rewrap it against the current viewport. The frame itself is painted by Area.
export function groupHeading(node,title,maxScale,measure,reservedWidth=44,reservedHeight=12,minHeight=0){
  const font='600 12px system-ui',widest=widestWord(title,font,measure);
  const lines=scale=>wrapText(title,node.width/scale-reservedWidth,font,measure);
  const fits=scale=>node.height/scale>=minHeight&&node.width/scale-reservedWidth>=widest&&node.height/scale-reservedHeight>=Math.max(20,lines(scale).length*16);
  let scale=maxScale;
  if(!fits(scale)){
    let low=0,high=scale;
    for(let i=0;i<20;i++){
      const middle=(low+high)/2;
      if(fits(middle))low=middle;else high=middle;
    }
    scale=low;
  }
  return {scale,title:lines(scale).join('\n')};
}

export function prepareCards(records, _inputOwner, measure, translate) {
  const wrap=(text,width,font)=>wrapText(text,width,font,measure);
  const byID=new Map(records.map(n=>[n.id,n]));
  // An input collection's inputs, through the part groups inside it.
  const leavesOf=id=>{const n=byID.get(id);return n?.children?.length?n.children.flatMap(leavesOf):[id];};
  const childNames=id=>(byID.get(id)?.children||[]).map(child=>byID.get(child)?.title).filter(Boolean);
  return records.map(n=>{
    const frame=!!n.children?.length;
    // A part's zoom button takes its room beside its title, an input's kind
    // mark its room before it.
    const title=wrap(n.title,!frame&&!n.activation&&n.symbols?.length?cardText-card.zoom:n.activation?cardText-kindMark:cardText,'700 17px system-ui');
    const label=wrap(n.title,152,'12px system-ui');
    const metadata=[n.language,n.componentKind?translate(n.componentKind):''].filter(Boolean).join(' · ');
    const roleLines=n.role?wrap(n.role,cardText,'600 13px system-ui'):[];
    // A part's name alone does not say what it is: its one-sentence
    // description stands under the name, at most three lines. The browser
    // wraps it in the text column; its lines are counted there for the
    // card's height and never inserted into the text.
    const part=!frame&&!n.activation&&!n.branch&&n.category==='part';
    const description=n.branch==='component'||n.category==='component'||part?n.summary||'':'';
    const descriptionMost=part?3:0;
    const descriptionLines=description?Math.min(descriptionMost||Infinity,wrap(description,cardText,'13px system-ui').length):0;
    const subtitle=n.category==='external'&&!n.title.endsWith(n.subtitle||'')?n.subtitle:'';
    const subtitleLines=subtitle?wrap(subtitle,cardText,'13px system-ui'):[];
    const names=n.branch==='component'?childNames(n.id):[];
    // External headings may use the full width below the zoom mark. Its extra
    // row is reserved by overviewHeading, not by forcing a wider frame.
    const input=!!n.activation,collection=['communication','inputs'].includes(n.branch);
    const inputGroups=n.branch==='inputs'?groupInputs(leavesOf(n.id).map(id=>byID.get(id)).filter(n=>n?.activation),translate):[];
    const widest=(text,font)=>widestWord(text,font,measure);
    // Reserve complete physical pixels. A fractional fit round-off at the
    // longest-word boundary must not unexpectedly add a row below the zoom mark.
    // A plain tile under its display group's heading needs room for its zoom
    // mark alone.
    // An input collection is headed by the glyph key's word: titled with its
    // component's name, Redis's inputs read as a second redis-server.
    const heading=n.branch==='inputs'?translate('Inputs'):n.branch==='outside'?translate('Outside'):'';
    // The Outside frame's summary is its chips: at the whole-map camera
    // they read at their own size, and the frame keeps their proportion.
    const grid=n.branch==='outside'?chipGrid((n.children||[]).length):null;
    const overviewMinWidth=grid?grid.width:Math.ceil(Math.max(widest(heading||n.title,collection?'700 13px system-ui':'700 18px system-ui')+(collection?16:64),
      ...names.map(name=>widest(name,'500 13px system-ui')+32),
      ...inputGroups.map(group=>widest(group.title,'500 13px system-ui')+32+kindMark)));
    // A short target name must not squeeze its area inventory into a column
    // of isolated words. Prefer two-line entries within the existing text
    // column; targets with short area names need no extra width.
    const twoLineWidth=text=>{
      const words=String(text).split(/\s+/).filter(Boolean);
      if(!words.length)return 0;
      return Math.min(...words.map((_,i)=>Math.max(
        measure(words.slice(0,i+1).join(' '),'500 13px system-ui'),
        measure(words.slice(i+1).join(' '),'500 13px system-ui'))));
    };
    const overviewPreferredWidth=grid?grid.width:Math.ceil(Math.max(overviewMinWidth,
      ...names.map(name=>32+Math.min(304,twoLineWidth(name)))));
    const overviewHeightAtWidth=grid?width=>width*grid.height/grid.width:['component','communication','inputs'].includes(n.branch)?(width,{availableHeight=Infinity}={})=>{
      const heading=overviewHeading(n,width,measure);
      // As the summary draws it (canvas.jsx ComponentOverview): inset 8px in
      // its frame and padded 8px, its heading, a 10px gap, and its kinds 6px
      // apart, each wrapped beside its kind mark. Counted 16px narrower
      // and without the gap, "Commands" had stood cut in half under Inputs.
      if(n.branch==='inputs'){
        const room=Math.max(1,Math.min(304,width-32)-kindMark),kinds=inputGroups.map(group=>wrap(group.title,room,'500 13px system-ui').length*18);
        return Math.max(44,33+Math.max(32,heading.height)+(kinds.length?10+kinds.reduce((a,b)=>a+b,0)+6*(kinds.length-1):0));
      }
      // The inventory remains complete in the scrollable summary. Its first
      // entrance sets the minimum usable height; fitting the entire list would
      // enlarge the world and make its fitted text smaller again.
      const rows=names.map(name=>10+wrap(name,Math.max(1,Math.min(304,width-32)),'500 13px system-ui').length*18);
      const base=(n.branch==='communication'?16:32)+heading.height;
      const list=rows.length?17+rows.reduce((sum,row)=>sum+row,0):0;
      const controlHeight=28+(n.branch==='communication'?16:32);
      return Math.max(controlHeight,base+(base+list>availableHeight&&rows.length?17+rows[0]:list));
    }:undefined;
    return {...n,heading,category:input?'input':n.category,name:n.title,title:title.join('\n'),labelTitle:label.join('\n'),inputGroups,metadata,role:roleLines.join('\n'),overviewHeightAtWidth,overviewMinWidth,overviewPreferredWidth,
      roleLabel:!input&&['core','triggers'].includes(n.lane)?translate(n.lane==='core'?'Core':'Entrypoints'):'',
      description,descriptionMost,subtitle:subtitleLines.join('\n'),
      labelWidth:180,labelHeight:label.length*16,
      headerHeight:Math.max(64,32+title.length*22+(metadata?24:0)+(roleLines.length?8+roleLines.length*18:0)+(descriptionLines?12+descriptionLines*18:0)),
      // An input says its kind by the mark before its name, in no row of its
      // own. A chip is the Outside frame's one size.
      width:n.branch==='chip'?chip.width:card.width,height:n.branch==='chip'?chip.height:frame?undefined:66-(input?24:0)+title.length*22+descriptionLines*18+
        (subtitleLines.length?8+subtitleLines.length*18:0)};
  });
}
