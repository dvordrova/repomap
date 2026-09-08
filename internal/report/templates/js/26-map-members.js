// The cubes are views of the key code already printed in each group. They
// neither extend the analysis graph nor attach group relations to a member.
var repomapMembers = (function () {
  var inventories = new WeakMap();
  function items(node) {
    if (inventories.has(node)) return inventories.get(node);
    var result = [], known = new Set();
    function add(item) {
      var key = item.source.Href || item.source.Open;
      if (!key || known.has(key)) return;
      known.add(key); result.push(item);
    }
    JSON.parse(node.dataset.concepts || '[]').forEach(add);
    var href = node.getAttribute('href') || '';
    var group = href[0] === '#' && document.getElementById(href.slice(1));
    if (group) group.querySelectorAll('.group-highlights .key-symbol').forEach(function (row) {
      var chip = row.querySelector('strong > .chip');
      if (!chip) return;
      var name = chip.cloneNode(true); name.querySelectorAll('.ln').forEach(function (n) { n.remove(); });
      var summary = row.querySelector('.model'), prose = summary && summary.cloneNode(true);
      if (prose) prose.querySelectorAll('.model-sources,.source-hint,button').forEach(function (n) { n.remove(); });
      var anchor = row.querySelector('.anchor');
      add({name:name.textContent.trim(), explanation:prose ? prose.textContent.trim() : '', source:{
        Href:chip.dataset.open ? '' : chip.getAttribute('href'), Open:chip.dataset.open || '',
        Text:anchor ? anchor.textContent : (chip.closest('.key-file').querySelector('.path').textContent + (chip.querySelector('.ln')?.textContent || ''))
      }});
    });
    // A part without interpreted highlights still has its original source
    // index. Showing those declarations is useful; inventing an explanation
    // or another intermediate group would not be.
    if(group&&!result.length)group.querySelectorAll('.symbol-index > li').forEach(function(row){
      var chip=row.querySelector('.chip');if(!chip)return;
      var name=chip.cloneNode(true);name.querySelectorAll('.ln').forEach(function(n){n.remove();});
      add({name:name.textContent.trim(),explanation:'',source:{Href:chip.dataset.open?'':chip.getAttribute('href'),Open:chip.dataset.open||'',Text:chip.getAttribute('title')||name.textContent.trim()}});
    });
    inventories.set(node, result); return result;
  }
  function size(node, availableWidth) {
    var list = items(node);
    if (!list.length) return {w:220,h:80};
    var width = Math.max(280, Math.min(560, availableWidth - 72));
    var columns = Math.max(1, Math.floor((width - 32) / 160));
    return {w:width,h:116 + Math.ceil(list.length / columns) * 62};
  }
  function label(node,item) {
    return item.name+(items(node).some(function(other){return other!==item&&other.name===item.name;})?' · '+item.source.Text:'');
  }
  function sourceLink(source) {
    var link=document.createElement(source.Href||source.Open?'a':'span');link.textContent=source.Text;
    if(source.Href){link.href=source.Href;link.target='_blank';link.rel='noopener';}
    else if(source.Open){link.href='#';link.dataset.open=source.Open;}
    return link;
  }
  function grid(map,node) {
    var grid = document.createElement('div'); grid.className='map-member-grid';
    items(node).forEach(function (item) {
      var b=document.createElement('button'); b.type='button'; b.className='map-member';
      b.textContent=item.name; b.title=(item.explanation ? item.explanation+'\n' : '')+item.source.Text;
      var duplicate=items(node).some(function(other){return other!==item&&other.name===item.name;});
      if(duplicate){
        var path=document.createElement('small');path.textContent=item.source.Text;b.appendChild(path);
      }
      b.dataset.memberSource=item.source.Href||item.source.Open;
      b.setAttribute('aria-label',rmT('Explain {0}',label(node,item)));
      b.setAttribute('aria-pressed','false');
      b.addEventListener('click',function(){
        grid.querySelectorAll('button').forEach(function(other){other.setAttribute('aria-pressed',other===b);});
        map.showMember(node,item);
      });
      grid.appendChild(b);
    });
    return grid;
  }
  function draw(map, node, box) {
    var svg = map.querySelector('svg'), previous = svg.querySelector('.map-member-layer');
    if (previous) previous.remove();
    if (!node || !box || !items(node).length) return;
    var layer = document.createElementNS('http://www.w3.org/2000/svg','foreignObject');
    layer.setAttribute('class','map-member-layer');
    layer.setAttribute('x',box.x + 16); layer.setAttribute('y',box.y + 70);
    layer.setAttribute('width',box.w - 32); layer.setAttribute('height',box.h - 78);
    var body = document.createElement('div'); body.className='map-member-content';
    var heading = document.createElement('div'); heading.className='map-member-label';
    heading.textContent=rmT('Key code')+' · '+items(node).length; body.appendChild(heading);
    body.appendChild(grid(map,node)); layer.appendChild(body); svg.appendChild(layer);
  }
  return {items:items,size:size,draw:draw,grid:grid,label:label,sourceLink:sourceLink};
})();
